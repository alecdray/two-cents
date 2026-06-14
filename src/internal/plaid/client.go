// Package plaid wraps the Plaid API as an external client. It translates
// Plaid's wire shapes into the app's provider-agnostic banking types and
// exposes a Service that satisfies banking.BankProvider. It is a leaf: no
// persistence, no domain imports, and no Plaid type escapes entities.go.
package plaid

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// errorCodeItemLoginRequired is the Plaid error_code returned when an Item's
// credentials have expired and the user must re-authenticate. It is mapped onto
// the provider-agnostic banking.ErrReauthRequired so consumers never depend on
// Plaid's native error vocabulary.
const errorCodeItemLoginRequired = "ITEM_LOGIN_REQUIRED"

// errorResponse mirrors the Plaid error envelope returned on a non-200 status.
// Only the fields used to classify the error are decoded.
type errorResponse struct {
	ErrorType string `json:"error_type"`
	ErrorCode string `json:"error_code"`
}

// defaultOrigin is Plaid's production base URL. Plaid also serves sandbox and
// development environments; the origin is configurable on the client so tests
// (and non-production deployments) can point it elsewhere.
const defaultOrigin = "https://production.plaid.com"

// defaultLanguage is the Link UI language sent on /link/token/create when the
// configuration leaves it unset.
const defaultLanguage = "en"

// defaultClientUserID identifies the end user on link-token requests when none
// is configured. This app is single-user with no per-user auth, so a fixed
// value suffices.
const defaultClientUserID = "two-cents"

// LinkConfig holds the app-level inputs Plaid's /link/token/create needs: the
// products the app requests access to, the country codes it operates in, the
// client name shown in the Link UI, the UI language, and the end-user id. These
// come from configuration and are fixed for the life of the client.
type LinkConfig struct {
	ClientName   string
	Language     string
	CountryCodes []string
	Products     []string
	ClientUserID string
}

func (l LinkConfig) language() string {
	if l.Language == "" {
		return defaultLanguage
	}
	return l.Language
}

func (l LinkConfig) clientUserID() string {
	if l.ClientUserID == "" {
		return defaultClientUserID
	}
	return l.ClientUserID
}

// Client owns the raw, authenticated HTTP calls to Plaid. Every request
// carries the app credentials (client_id + secret) plus a per-Item
// access_token (the bank login).
type Client struct {
	clientID   string
	secret     string
	origin     string
	httpClient *http.Client
	link       LinkConfig
}

// ClientOpt customizes a Client at construction.
type ClientOpt func(*Client)

// WithOrigin overrides the Plaid base URL (e.g. the sandbox host, or a test
// server). An empty value is ignored.
func WithOrigin(origin string) ClientOpt {
	return func(c *Client) {
		if origin != "" {
			c.origin = origin
		}
	}
}

// WithLinkConfig sets the app-level configuration Plaid's link/token/create
// needs (products, country codes, client name, language, user id).
func WithLinkConfig(link LinkConfig) ClientOpt {
	return func(c *Client) {
		c.link = link
	}
}

// WithHTTPClient overrides the underlying HTTP client (e.g. to inject a
// round-tripper that captures outgoing requests in tests). A nil value is
// ignored.
func WithHTTPClient(httpClient *http.Client) ClientOpt {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// NewClient builds a Plaid client from the app credentials. It fails fast when
// either credential is blank rather than later issuing an unauthenticated
// request.
func NewClient(clientID, secret string, opts ...ClientOpt) (*Client, error) {
	if clientID == "" {
		return nil, errors.New("clientID cannot be empty")
	}
	if secret == "" {
		return nil, errors.New("secret cannot be empty")
	}
	c := &Client{
		clientID:   clientID,
		secret:     secret,
		origin:     defaultOrigin,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// credentials are the auth fields every Plaid request body carries. They are
// embedded into each endpoint's request struct so they appear inline in the
// JSON body Plaid expects. AccessToken is omitempty because some endpoints
// authenticate with the app credentials alone (creating a new link token,
// exchanging a public token), and a new-connection link token must not carry
// one at all.
type credentials struct {
	ClientID    string `json:"client_id"`
	Secret      string `json:"secret"`
	AccessToken string `json:"access_token,omitempty"`
}

// post sends a JSON request to a Plaid endpoint with the app credentials and
// the per-Item access token injected, and decodes the JSON response into out.
func (c *Client) post(ctx contextx.ContextX, path, accessToken string, body, out any) error {
	payload, err := mergeCredentials(body, credentials{
		ClientID:    c.clientID,
		Secret:      c.secret,
		AccessToken: accessToken,
	})
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		var errResp errorResponse
		if json.Unmarshal(msg, &errResp) == nil && errResp.ErrorCode == errorCodeItemLoginRequired {
			return fmt.Errorf("plaid item login required: %w", banking.ErrReauthRequired)
		}
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(msg))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}

// mergeCredentials serializes body and the auth credentials into a single flat
// JSON object, so the credentials sit alongside the endpoint's own fields.
func mergeCredentials(body any, creds credentials) ([]byte, error) {
	fields := map[string]json.RawMessage{}

	credBytes, err := json.Marshal(creds)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(credBytes, &fields); err != nil {
		return nil, err
	}

	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(bodyBytes, &fields); err != nil {
			return nil, err
		}
	}

	return json.Marshal(fields)
}

// getAccounts issues /accounts/get for the given bank login.
func (c *Client) getAccounts(ctx contextx.ContextX, accessToken string) (*accountsResponse, error) {
	var out accountsResponse
	if err := c.post(ctx, "/accounts/get", accessToken, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// getBalances issues /accounts/balance/get, which returns live balances for
// the bank login's accounts.
func (c *Client) getBalances(ctx contextx.ContextX, accessToken string) (*accountsResponse, error) {
	var out accountsResponse
	if err := c.post(ctx, "/accounts/balance/get", accessToken, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// syncTransactions issues a single /transactions/sync page from the given
// cursor (empty = from the beginning).
func (c *Client) syncTransactions(ctx contextx.ContextX, accessToken, cursor string) (*transactionsSyncResponse, error) {
	var out transactionsSyncResponse
	if err := c.post(ctx, "/transactions/sync", accessToken, transactionsSyncRequest{Cursor: cursor}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// createLinkToken issues /link/token/create. With an empty accessToken it
// requests a token for a new connection, sending the configured products; with
// an access token it requests an update-mode token to reconnect that login,
// which Plaid requires to carry the access_token and omit products.
func (c *Client) createLinkToken(ctx contextx.ContextX, accessToken string) (*linkTokenCreateResponse, error) {
	body := linkTokenCreateRequest{
		ClientName:   c.link.ClientName,
		Language:     c.link.language(),
		CountryCodes: c.link.CountryCodes,
		User:         linkUser{ClientUserID: c.link.clientUserID()},
	}
	if accessToken == "" {
		body.Products = c.link.Products
	}

	var out linkTokenCreateResponse
	if err := c.post(ctx, "/link/token/create", accessToken, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// exchangePublicToken issues /item/public_token/exchange, trading the public
// token from a completed connect flow for a durable access token and item id.
// It authenticates with the app credentials alone — no access token yet exists.
func (c *Client) exchangePublicToken(ctx contextx.ContextX, publicToken string) (*publicTokenExchangeResponse, error) {
	var out publicTokenExchangeResponse
	if err := c.post(ctx, "/item/public_token/exchange", "", publicTokenExchangeRequest{PublicToken: publicToken}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// removeItem issues /item/remove for the given bank login, invalidating its
// access token at Plaid.
func (c *Client) removeItem(ctx contextx.ContextX, accessToken string) error {
	var out itemRemoveResponse
	return c.post(ctx, "/item/remove", accessToken, struct{}{}, &out)
}

// ensure contextx.ContextX satisfies context.Context for request building.
var _ context.Context = contextx.ContextX{}
