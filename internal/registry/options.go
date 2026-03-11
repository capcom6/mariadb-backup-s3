package registry

type options struct {
	withRecovery bool
}

type Option func(*options)

func WithRecovery() Option {
	return func(o *options) {
		o.withRecovery = true
	}
}
