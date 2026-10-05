package eventcatalog

// Config is EVENTCATALOG_* settings, embedded into config.Config by the
// composition root with envPrefix "EVENTCATALOG_". The module has no
// tunables yet; the struct exists so the registry line has its usual shape.
type Config struct{}
