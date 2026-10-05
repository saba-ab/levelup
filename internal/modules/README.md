# Modules

Business modules live here, one directory each, implementing `modkit.Module`.
The template ships with none. Scaffold one with `task new-module NAME=<name>`,
then add its constructor line in `internal/app/registry.go`, its `Config`
embed in `internal/config/config.go`, and its name to `MODULES_ENABLED`.

`docs/examples.md` is the cookbook: real excerpts of the three reference
modules that used to live here, covering every pattern the platform
supports — ports and adapters, subscriptions, jobs, resource-scoped
authorization, row locking, migrations with permission seeds, the auth
flow, the cached repository, the thin-module shape, and the test shapes.
