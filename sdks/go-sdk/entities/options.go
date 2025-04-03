package entities

// Option is a function that configures an Entity.
type Option func(*Entity) error

// WithDebug returns an Option that enables or disables debug mode for the Entity.
func WithDebug(debug bool) Option {
	return func(e *Entity) error {
		e.httpClient.debug = debug
		return nil
	}
}

// WithUserAgent returns an Option that sets the user agent for the Entity.
func WithUserAgent(userAgent string) Option {
	return func(e *Entity) error {
		e.httpClient.userAgent = userAgent
		return nil
	}
}
