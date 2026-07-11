package app

// App is the cloud service composition root.
type App struct {
	name string
}

// New constructs the cloud application.
func New(name string) *App {
	return &App{name: name}
}

// Name returns the stable service name.
func (a *App) Name() string {
	return a.name
}
