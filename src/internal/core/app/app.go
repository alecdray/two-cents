package app

// App is the application handle threaded through requests and tasks. It holds
// the loaded Config. Auth claims will be added here when the single local
// login lands (see ADR-0001 — single-user, no third-party OAuth).
type App struct {
	config Config
}

func NewApp(config Config) App {
	return App{config: config}
}

func (app App) Config() Config {
	return app.config
}
