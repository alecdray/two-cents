package schedule

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
)

// ValidationError is a recoverable, user-facing input error (a nameless item, a
// cadence with no date to go with it). Adapters surface its Message inline
// rather than treating it as a server failure, the same shape budget uses.
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string { return e.Message }

// maxNameLen bounds a declared item's name so a stray paste cannot make the
// schedule unreadable. It matches the account custom-name bound.
const maxNameLen = 60

// Service owns the sweep schedule — the declared recurring checking activity the
// cash-flow timeline is built from. It writes only its own table and reads no
// other module.
type Service struct {
	db *db.DB
}

// NewService builds a schedule Service over the database.
func NewService(d *db.DB) *Service {
	return &Service{db: d}
}

// repo binds a Repo to the global (non-transactional) query handle.
func (s *Service) repo() *Repo {
	return NewRepo(s.db.Queries())
}

// List returns the whole declared schedule, inactive items included — what the
// management surface renders.
func (s *Service) List(ctx contextx.ContextX) ([]Item, error) {
	items, err := s.repo().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("schedule: list: %w", err)
	}
	return items, nil
}

// ActiveItems returns only the items that currently belong on the timeline. It
// is the read seam the sweep consumes: an inactive item is kept in storage but
// contributes nothing, so deactivating is how a user retires a commitment
// without losing what they declared.
func (s *Service) ActiveItems(ctx contextx.ContextX) ([]Item, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	active := make([]Item, 0, len(all))
	for _, item := range all {
		if item.Active {
			active = append(active, item)
		}
	}
	return active, nil
}

// Create validates and stores a new Item, returning it with the id it was
// assigned. An invalid declaration is a ValidationError and nothing is stored.
func (s *Service) Create(ctx contextx.ContextX, item Item) (Item, error) {
	normalized, err := normalize(item)
	if err != nil {
		return Item{}, err
	}
	normalized.ID = uuid.NewString()
	if err := s.repo().Insert(ctx, normalized); err != nil {
		return Item{}, fmt.Errorf("schedule: create: %w", err)
	}
	return normalized, nil
}

// Update validates and overwrites the stored Item with the same id. As with
// Create, an invalid declaration changes nothing.
func (s *Service) Update(ctx contextx.ContextX, item Item) error {
	normalized, err := normalize(item)
	if err != nil {
		return err
	}
	if item.ID == "" {
		return ValidationError{Message: "That scheduled item can't be found."}
	}
	normalized.ID = item.ID
	if err := s.repo().Update(ctx, normalized); err != nil {
		return fmt.Errorf("schedule: update: %w", err)
	}
	return nil
}

// Delete removes the Item with the given id from the schedule.
func (s *Service) Delete(ctx contextx.ContextX, id string) error {
	if err := s.repo().Delete(ctx, id); err != nil {
		return fmt.Errorf("schedule: delete: %w", err)
	}
	return nil
}

// normalize trims the name and validates the declaration, returning the Item as
// it should be stored. The date fields are cleared for the cadence that does not
// use them, so a stored row never describes a cadence it has no date for.
//
// The amount is required to be positive: direction carries the sign, and a
// negative amount paired with a direction would let one declaration mean two
// opposite things.
func normalize(item Item) (Item, error) {
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" {
		return Item{}, ValidationError{Message: "Give the scheduled item a name."}
	}
	if len(item.Name) > maxNameLen {
		return Item{}, ValidationError{Message: fmt.Sprintf("Keep the name to %d characters or fewer.", maxNameLen)}
	}
	if item.Direction != DirectionOut && item.Direction != DirectionIn {
		return Item{}, ValidationError{Message: "Choose whether the money goes out or comes in."}
	}
	if item.Amount <= 0 {
		return Item{}, ValidationError{Message: "Enter an amount greater than zero."}
	}

	switch item.Cadence {
	case CadenceMonthly:
		if item.DayOfMonth < 1 || item.DayOfMonth > 31 {
			return Item{}, ValidationError{Message: "Choose a day of the month between 1 and 31."}
		}
		item.AnchorDate = time.Time{}
	case CadenceBiweekly:
		if item.AnchorDate.IsZero() {
			return Item{}, ValidationError{Message: "Give a date the item last fell on, so the every-14-days cadence has an anchor."}
		}
		item.DayOfMonth = 0
	default:
		return Item{}, ValidationError{Message: "Choose a monthly or every-two-weeks cadence."}
	}

	return item, nil
}

// IsValidationError reports whether err is (or wraps) a ValidationError, so
// adapters can render its message inline instead of returning a server error.
func IsValidationError(err error) (ValidationError, bool) {
	var ve ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return ValidationError{}, false
}
