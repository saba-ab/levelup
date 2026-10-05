# Module cookbook

The template ships no business modules. This document is what the three
reference modules that used to live in `internal/modules/` demonstrated,
kept as excerpts so the shapes survive without the code: `user` (rich,
fully layered, owns auth), `wallet` (both dependency directions, resource
authorization, row locking, a reconciling job) and `notification` (thin and
flat). Every snippet below is real code that passed `task check` in this
repository; only comments were trimmed.

Start a module with `task new-module NAME=<name>`. The scaffold gives you
the skeleton in section 1; the rest of this document is what you fill in.
The rules the snippets obey are in `CLAUDE.md`; the reasons are in the PRD.

## 1. Anatomy of a module

```text
internal/modules/wallet/
  module.go          implements modkit.Module; the only importable file besides contracts/ and config.go
  config.go          WALLET_* settings, embedded into config.Config by the composition root
  ports.go           type aliases re-exporting internal/ports for the registry
  contracts/         topics, event payloads, permission catalogue, offered Reader interfaces. Data only.
  adapters/          bridges this module's ports to a provider's contracts (local in-process, or remote HTTP)
  internal/domain    entities and invariants; no gorm, validate, or json tags
  internal/app       services: transaction boundaries, resource-scoped authz, every outbox publish
  internal/ports     port interfaces plus the module's own snapshot types
  internal/repo      unexported GORM models with toDomain/fromDomain
  internal/transport chi handlers, DTOs with validate tags, swag annotations
  migrations/        goose SQL plus typed Go seeds, applied into schema wallet_svc
```

The thin shape (`notification`) collapses `internal/` into three flat files:
`repo.go`, `service.go`, `http.go`. The rule is about what escapes the
package, not how many packages there are.

## 2. `module.go`

Wallet's, showing every method of `modkit.Module` doing something.

```go
package wallet

type Module struct {
	svc  *app.Service
	repo *repo.Postgres
	deps modkit.Deps
	cfg  Config
}

// New wires the module. users is the port implementation the composition
// root chose: adapters.NewLocalUserReader(userMod.Reader()) today — swap
// that ONE registry line for adapters.NewRemoteUserReader(url, timeout)
// and the module is extracted. Nothing below changes.
func New(d modkit.Deps, cfg Config, users ports.UserReader) *Module {
	r := repo.NewPostgres(d.DB)
	svc := app.NewService(r, users, d.Outbox, d.Authz, d.DB, d.Clock)
	return &Module{svc: svc, repo: r, deps: d, cfg: cfg}
}

func (m *Module) Name() string { return "wallet" }

func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: typed seeding of the
// permission catalogue.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

// Subscriptions: wallet reacts to user.registered.v1 by opening the user's
// wallet — idempotent by construction (unique (user_id, currency) + upsert).
func (m *Module) Subscriptions() []bus.Subscription {
	return []bus.Subscription{{
		Topic:   usercontracts.TopicUserRegistered,
		Group:   "wallet",
		Handler: m.onUserRegistered,
	}}
}

func (m *Module) onUserRegistered(ctx context.Context, e bus.Envelope) error {
	var ev usercontracts.UserRegisteredV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		// errs.Invalid → the consumer loop dead-letters instead of retrying.
		return errs.Wrap(errs.Invalid, "undecodable user.registered.v1", err)
	}
	_, _, err := m.svc.CreateForUser(ctx, ev.UserID, ev.Currency)
	return err
}

func (m *Module) Jobs() []jobs.Job {
	return []jobs.Job{{
		Name:     "wallet.reconcile",
		Schedule: m.cfg.ReconcileSchedule,
		Run: func(ctx context.Context, _ []byte) error {
			return m.svc.Reconcile(ctx, m.repo)
		},
	}}
}

func (m *Module) Health(ctx context.Context) error {
	sqlDB, err := m.deps.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (m *Module) Permissions() []authz.Permission { return walletcontracts.AllPermissions }
```

Constructors do no I/O. Every binary builds the whole registry, so a
constructor that dials something makes `cmd/migrate` need a broker.

## 3. Config and the registry line

The module owns its settings struct. The composition root embeds it.

```go
// internal/modules/wallet/config.go
package wallet

type Config struct {
	// ReconcileSchedule is the cron spec for the reconciling sweep (R47).
	ReconcileSchedule string `env:"RECONCILE_SCHEDULE" envDefault:"*/15 * * * *"`
	// UserServiceURL flips the user port to the remote adapter: the one
	// config value extraction changes. Empty → in-process local adapter.
	UserServiceURL string `env:"USER_URL"`
}
```

```go
// internal/config/config.go
	User   user.Config   `envPrefix:"USER_"`
	Wallet wallet.Config `envPrefix:"WALLET_"`
```

```go
// internal/app/registry.go
	all := map[string]func() modkit.Module{
		"user": func() modkit.Module {
			userMod = user.New(p.DepsFor("user"), cfg.User, p.Authn)
			return userMod
		},
		"wallet": func() modkit.Module {
			// THE extraction seam: local adapter while user runs in-process;
			// set WALLET_USER_URL and drop user from MODULES_ENABLED to
			// consume it remotely instead. Nothing inside wallet changes.
			var users wallet.UserReader
			switch {
			case cfg.Wallet.UserServiceURL != "":
				users = adapters.NewRemoteUserReader(cfg.Wallet.UserServiceURL, 2*time.Second)
			case userMod != nil:
				users = adapters.NewLocalUserReader(userMod.Reader())
			default:
				// Fail fast at boot with an actionable message (R2).
				panic(`wallet needs the user module: enable "user" BEFORE "wallet" in MODULES_ENABLED, or set WALLET_USER_URL for remote mode`)
			}
			return wallet.New(p.DepsFor("wallet"), cfg.Wallet, users)
		},
		"notification": func() modkit.Module { return notification.New(p.DepsFor("notification")) },
	}
```

`user` took `p.Authn` as an explicit third argument because it owns
credentials. Module-specific wiring stays visible in the registry on
purpose; that is why the registry is hand-written and not generated.

## 4. `contracts/`: the public surface

Data only, zero dependencies beyond platform contract types. Producers own
event schemas; consumers own the interfaces they need.

```go
// contracts/topics.go
// Topic names carry the schema version: breaking changes mean a new .v2
// topic with dual-publish, never a silent field repurpose.
const TopicUserRegistered = "user.registered.v1"

// contracts/events.go
// UserRegisteredV1 is the authoritative schema for user.registered.v1 —
// producers own event schemas, consumers import them. Additive changes
// only; new fields must be optional.
type UserRegisteredV1 struct {
	UserID   string    `json:"user_id"`
	Email    string    `json:"email"`
	Currency string    `json:"currency"`
	At       time.Time `json:"at"`
}

// contracts/permissions.go
// Module-owned permission catalogue: the seed migration and the
// enforcement read these same constants, so the database can never grant
// a permission the code does not define.
var (
	PermWithdraw = authz.Permission{Module: "wallet", Action: "withdraw"}
	PermViewAny  = authz.Permission{Module: "wallet", Action: "view_any"}

	AllPermissions = []authz.Permission{PermWithdraw, PermViewAny}
)

// contracts/types.go
type UserID string
type Status string

const (
	StatusActive  Status = "active"
	StatusBlocked Status = "blocked"
)

// User is the projection other modules may read through Reader. It is NOT
// the domain entity — consumers own their own snapshot types on top of this.
type User struct {
	ID       UserID
	Email    string
	Status   Status
	Currency string
}

// Reader is user's offered synchronous read surface. It lives in contracts
// so consumer ADAPTERS may reference it — consumers still declare their
// OWN ports and map User into their own snapshot types.
type Reader interface {
	Snapshot(ctx context.Context, id string) (User, error)
	SnapshotByIDs(ctx context.Context, ids []string) (map[string]User, error)
}
```

The module exposes the reader from `module.go`, and the service implements it:

```go
func (m *Module) Reader() contracts.Reader { return m.svc }
```

## 5. Domain: invariants, no tags

```go
package domain

type Wallet struct {
	ID       contracts.WalletID
	UserID   string // bare uuid — NO foreign key to user_svc (P5)
	Balance  money.Amount
	Currency string
	// Version is the optimistic-concurrency token; the repository refuses a
	// write when it moved underneath the transaction.
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewWallet(userID, currency string, now time.Time) (Wallet, error) {
	if userID == "" {
		return Wallet{}, ErrNoOwner
	}
	if len(currency) != 3 {
		return Wallet{}, ErrBadCurrency
	}
	return Wallet{
		ID:        contracts.WalletID(id.NewID()),
		UserID:    userID,
		Currency:  currency,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Withdraw refuses overdrafts: "a withdrawal may not exceed the balance" is
// exactly the class of rule that lives here, never in a validate tag.
func (w *Wallet) Withdraw(amount money.Amount, now time.Time) error {
	if amount <= 0 {
		return ErrNonPositiveAmount
	}
	if amount > w.Balance {
		return ErrInsufficientFunds
	}
	next, err := w.Balance.Sub(amount)
	if err != nil {
		return err
	}
	w.Balance = next
	w.UpdatedAt = now
	return nil
}

func (w *Wallet) OwnedBy(userID string) bool { return w.UserID == userID }
```

Errors are package-level values with a kind. The kind decides the HTTP
status and, in a subscriber, whether the consumer loop retries or parks.

```go
var (
	ErrNoOwner           = errs.New(errs.Invalid, "wallet needs an owner")
	ErrBadCurrency       = errs.New(errs.Invalid, "currency must be a 3-letter code")
	ErrNonPositiveAmount = errs.New(errs.Invalid, "amount must be positive")
	ErrInsufficientFunds = errs.New(errs.Conflict, "insufficient funds")
	ErrNotFound          = errs.New(errs.NotFound, "wallet not found")
	ErrVersionConflict   = errs.New(errs.Conflict, "wallet modified concurrently, retry")
	ErrCurrencyMismatch  = errs.New(errs.Invalid, "currency does not match the owner's")
	ErrOwnerInactive     = errs.New(errs.Conflict, "owner is not active")
)
```

`user.NewUser` normalises (lower-cases the email, upper-cases the currency)
before checking, so the entity carries canonical values and the unique
index on `email` means what it says.

## 6. Service: transactions, authorization, outbox

The service owns transaction boundaries and every publish. The `tx` runner
is a field so unit tests can inject a pass-through and need no database.

```go
package app

// Repository is consumed by this service, implemented in internal/repo.
// Create reports whether a row was actually inserted (idempotent creates).
type Repository interface {
	Create(ctx context.Context, tx *gorm.DB, w domain.Wallet) (bool, error)
	ByID(ctx context.Context, id string) (domain.Wallet, error)
	ByOwner(ctx context.Context, userID, currency string) (domain.Wallet, error)
	// ByIDForUpdate takes a row lock (SELECT ... FOR UPDATE) — money
	// movements serialize on the wallet row.
	ByIDForUpdate(ctx context.Context, tx *gorm.DB, id string) (domain.Wallet, error)
	Save(ctx context.Context, tx *gorm.DB, w domain.Wallet) error
}

type Service struct {
	repo   Repository
	users  ports.UserReader
	outbox outbox.Store
	authz  authz.Enforcer
	db     *gorm.DB
	clock  clock.Clock

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

func NewService(repo Repository, users ports.UserReader, ob outbox.Store,
	enf authz.Enforcer, db *gorm.DB, c clock.Clock) *Service {
	s := &Service{repo: repo, users: users, outbox: ob, authz: enf, db: db, clock: c}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}
```

**Idempotent create for a subscriber.** The event handler calls this; a
redelivery returns the existing row instead of a fresh one.

```go
// CreateForUser opens a wallet for a user, validating against wallet's OWN
// snapshot of the user (through the port — never user's internals).
// Returns created=false when the wallet already exists: the
// user.registered.v1 subscriber must be idempotent.
func (s *Service) CreateForUser(ctx context.Context, userID, currency string) (domain.Wallet, bool, error) {
	snap, err := s.users.ByID(ctx, userID)
	if err != nil {
		return domain.Wallet{}, false, err
	}
	if !snap.Active() {
		return domain.Wallet{}, false, domain.ErrOwnerInactive
	}
	if currency == "" {
		currency = snap.Currency
	}
	if currency != snap.Currency {
		return domain.Wallet{}, false, domain.ErrCurrencyMismatch
	}

	w, err := domain.NewWallet(userID, currency, s.clock.Now())
	if err != nil {
		return domain.Wallet{}, false, err
	}

	var created bool
	err = s.tx(ctx, func(tx *gorm.DB) error {
		created, err = s.repo.Create(ctx, tx, w)
		return err
	})
	if err != nil {
		return domain.Wallet{}, false, err
	}
	if !created {
		// The insert hit the (user, currency) conflict: a wallet already
		// exists. Return THAT one — the freshly minted w carries an ID no
		// row has, and handing it back makes every follow-up 404.
		existing, err := s.repo.ByOwner(ctx, userID, currency)
		if err != nil {
			return domain.Wallet{}, false, err
		}
		return existing, false, nil
	}
	return w, true, nil
}
```

**Resource-scoped authorization plus a money movement.** The role gate and
ownership must both hold. Ownership can only be checked here, where the
entity is loaded. The row lock serializes concurrent movements. The event
rides the same transaction.

```go
func (s *Service) Withdraw(ctx context.Context, p authz.Principal, walletID string, amount money.Amount) (domain.Wallet, error) {
	if err := s.authz.Authorize(ctx, p, contracts.PermWithdraw, nil); err != nil {
		return domain.Wallet{}, err
	}

	var out domain.Wallet
	err := s.tx(ctx, func(tx *gorm.DB) error {
		w, err := s.repo.ByIDForUpdate(ctx, tx, walletID)
		if err != nil {
			return err
		}
		if !w.OwnedBy(p.UserID) {
			return errs.New(errs.PermissionDenied, "not your wallet")
		}
		if err := w.Withdraw(amount, s.clock.Now()); err != nil {
			return err
		}
		if err := s.repo.Save(ctx, tx, w); err != nil {
			return err
		}
		out = w
		return s.outbox.Publish(ctx, tx, contracts.TopicWalletDebited, contracts.WalletDebitedV1{
			WalletID:    string(w.ID),
			UserID:      w.UserID,
			AmountMinor: amount.Minor(),
			Currency:    w.Currency,
			At:          s.clock.Now(),
		})
	})
	if err != nil {
		return domain.Wallet{}, err
	}
	return out, nil
}

// Get authorizes in the service: owners always may; anyone else needs
// wallet:view_any — the loaded entity decides.
func (s *Service) Get(ctx context.Context, p authz.Principal, walletID string) (domain.Wallet, error) {
	w, err := s.repo.ByID(ctx, walletID)
	if err != nil {
		return domain.Wallet{}, err
	}
	if !w.OwnedBy(p.UserID) {
		if err := s.authz.Authorize(ctx, p, contracts.PermViewAny, w); err != nil {
			return domain.Wallet{}, err
		}
	}
	return w, nil
}
```

**A state change that publishes.** User's registration: the row and the
event commit or roll back together.

```go
func (s *Service) Register(ctx context.Context, cmd RegisterCmd) (domain.User, error) {
	u, err := domain.NewUser(cmd.Email, cmd.Currency, s.clock.Now())
	if err != nil {
		return domain.User{}, err
	}
	u.PasswordHash, err = hashPassword(cmd.Password)
	if err != nil {
		return domain.User{}, err
	}

	err = s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.Create(ctx, tx, u); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicUserRegistered, contracts.UserRegisteredV1{
			UserID:   string(u.ID),
			Email:    u.Email,
			Currency: u.Currency,
			At:       s.clock.Now(),
		})
	})
	if err != nil {
		return domain.User{}, err
	}
	return u, nil
}
```

**A reconciling job.** Sweeps everything since the last successful run, so a
skipped tick leaves no permanent gap.

```go
type Reconciler interface {
	SweepSince(ctx context.Context, since time.Time) (int64, error)
	LastRun(ctx context.Context) (time.Time, error)
	MarkRun(ctx context.Context, at time.Time) error
}

func (s *Service) Reconcile(ctx context.Context, rec Reconciler) error {
	since, err := rec.LastRun(ctx)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	if _, err := rec.SweepSince(ctx, since); err != nil {
		return err
	}
	return rec.MarkRun(ctx, now)
}
```

## 7. Repository: unexported models, idempotent insert, row lock, version

```go
package repo

// wallet derives table name "wallets"; the module session prefixes it to
// wallet_svc.wallets. NO TableName() override — TablePrefix ignores those.
type wallet struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	UserID       string `gorm:"type:uuid;not null;uniqueIndex:idx_wallets_owner_currency"` // bare uuid, NO FK
	BalanceMinor int64  `gorm:"not null"`
	Currency     string `gorm:"not null;uniqueIndex:idx_wallets_owner_currency"`
	Version      int    `gorm:"not null;default:0"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (m wallet) toDomain() domain.Wallet { /* field by field */ }
func fromDomain(w domain.Wallet) wallet  { /* field by field */ }

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

// Create inserts unless a wallet for (user, currency) exists — the
// subscriber's idempotency by construction.
func (r *Postgres) Create(ctx context.Context, tx *gorm.DB, w domain.Wallet) (bool, error) {
	m := fromDomain(w)
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "currency"}},
			DoNothing: true,
		}).
		Create(&m)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert wallet", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) ByID(ctx context.Context, id string) (domain.Wallet, error) {
	var m wallet
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Wallet{}, domain.ErrNotFound
	case err != nil:
		return domain.Wallet{}, errs.Wrap(errs.Internal, "load wallet", err)
	}
	return m.toDomain(), nil
}

// ByIDForUpdate takes the row lock. Concurrent movements on one wallet
// serialize here.
func (r *Postgres) ByIDForUpdate(ctx context.Context, tx *gorm.DB, id string) (domain.Wallet, error) {
	var m wallet
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&m, "id = ?", id).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Wallet{}, domain.ErrNotFound
	case err != nil:
		return domain.Wallet{}, errs.Wrap(errs.Internal, "lock wallet", err)
	}
	return m.toDomain(), nil
}

// Save writes back guarded by the optimistic version — belt to the row
// lock's braces: a write from a stale read fails instead of clobbering.
func (r *Postgres) Save(ctx context.Context, tx *gorm.DB, w domain.Wallet) error {
	m := fromDomain(w)
	res := tx.WithContext(ctx).Model(&wallet{}).
		Where("id = ? AND version = ?", m.ID, m.Version).
		Updates(map[string]any{
			"balance_minor": m.BalanceMinor,
			"updated_at":    m.UpdatedAt,
			"version":       gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "save wallet", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}
```

Translating a unique violation into a domain error, from `user`:

```go
func (r *Postgres) Create(ctx context.Context, tx *gorm.DB, u domain.User) error {
	m := fromDomain(u)
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		if isUniqueViolation(err) {
			return domain.ErrEmailTaken
		}
		return errs.Wrap(errs.Internal, "insert user", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
```

The reconcile marker the job in section 6 uses:

```go
type reconcileMarker struct {
	ID      int `gorm:"primaryKey"`
	LastRun time.Time
}

func (r *Postgres) LastRun(ctx context.Context) (time.Time, error) {
	var m reconcileMarker
	err := r.db.WithContext(ctx).First(&m, "id = 1").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, nil // never ran: sweep everything
	}
	if err != nil {
		return time.Time{}, errs.Wrap(errs.Internal, "load reconcile marker", err)
	}
	return m.LastRun, nil
}

func (r *Postgres) MarkRun(ctx context.Context, at time.Time) error {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"last_run"}),
		}).
		Create(&reconcileMarker{ID: 1, LastRun: at}).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "mark reconcile run", err)
	}
	return nil
}
```

## 8. Transport: routes, auth groups, DTOs, swag

Every wallet route requires a principal. User mixes public and protected
groups, and exposes the internal surface remote adapters consume after
extraction.

```go
// wallet
func (h *Handler) Mount(r chi.Router) {
	r.Route("/wallets", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Post("/", h.create)
		r.Get("/{id}", h.get)
		r.Post("/{id}/deposit", h.deposit)
		r.Post("/{id}/withdraw", h.withdraw)
	})
}

// user
func (h *Handler) Mount(r chi.Router) {
	r.Route("/users", func(r chi.Router) {
		r.Post("/", h.register)
		r.Get("/{id}", h.get)
	})
	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", h.login)
		r.Post("/refresh", h.refresh)
		r.Group(func(r chi.Router) {
			r.Use(httpx.RequireAuth)
			r.Post("/logout", h.logout)
		})
	})
	// Internal surface consumed by other modules' remote adapters after
	// extraction. Same process serves it today; a separate binary serves
	// it tomorrow — the consumer cannot tell the difference.
	r.Get("/internal/users", h.batch)
}
```

Before you copy the internal route: it is mounted without authentication.
Give it a service-to-service credential or a separate listener before any
shared environment.

DTOs validate shape only. Invariants stay in constructors.

```go
type CreateReq struct {
	Currency string `json:"currency" validate:"omitempty,iso4217"`
}

type MoveReq struct {
	AmountMinor int64 `json:"amount_minor" validate:"required,gt=0"`
}

type RegisterReq struct {
	Email    string `json:"email"    validate:"required,email"`
	Currency string `json:"currency" validate:"required,iso4217"`
	Password string `json:"password" validate:"required,min=8,max=128"`
}
```

A handler decodes, calls the service, renders. Errors go through
`httpx.Error`, which maps the kind to a status and hides 5xx detail. The
swag annotations are what the OpenAPI drift test checks.

```go
// @Summary      Open a wallet for the authenticated user
// @Tags         wallet
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateReq true "Options"
// @Success      201 {object} WalletResp
// @Failure      401 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Router       /wallets [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, _ := authz.From(r.Context())
	wallet, created, err := h.svc.CreateForUser(r.Context(), p.UserID, req.Currency)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK // idempotent re-create returns the existing state
	}
	httpx.JSON(w, status, toResp(wallet))
}
```

## 9. Migrations: SQL plus typed seeds

```sql
-- migrations/0001_init.sql
-- +goose Up
CREATE TABLE wallets (
    id            UUID PRIMARY KEY,
    -- Bare uuid, deliberately NO foreign key to user_svc.users (P5).
    user_id       UUID NOT NULL,
    balance_minor BIGINT NOT NULL DEFAULT 0,
    currency      TEXT NOT NULL,
    version       INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
);

-- One wallet per (owner, currency); also what makes the user.registered.v1
-- subscriber idempotent by construction (ON CONFLICT DO NOTHING).
CREATE UNIQUE INDEX idx_wallets_owner_currency ON wallets (user_id, currency);

CREATE TABLE reconcile_markers (
    id       INT PRIMARY KEY,
    last_run TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE reconcile_markers;
DROP TABLE wallets;
```

The permission seed reads the same constants the enforcement reads, and the
default grant is the one line that made wallets usable without a role admin
API. Ownership is still checked in the service.

```go
// migrations/0002_seed_permissions.go
func Go() []*goose.Migration {
	return []*goose.Migration{
		goose.NewGoMigration(2,
			&goose.GoFunc{RunTx: upSeedPermissions},
			&goose.GoFunc{RunTx: downSeedPermissions},
		),
		goose.NewGoMigration(3,
			&goose.GoFunc{RunTx: upSeedDefaultGrant},
			&goose.GoFunc{RunTx: downSeedDefaultGrant},
		),
	}
}

const DefaultRoleID = 1

func upSeedPermissions(ctx context.Context, tx *sql.Tx) error {
	for _, p := range contracts.AllPermissions {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO authz_svc.permissions (key, module) VALUES ($1, $2)
			 ON CONFLICT (key) DO NOTHING`, p.Key(), p.Module); err != nil {
			return err
		}
	}
	return nil
}

func downSeedPermissions(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM authz_svc.permissions WHERE module = 'wallet'`)
	return err
}

func upSeedDefaultGrant(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO authz_svc.casbin_rule (ptype, v0, v1) VALUES ('p', $1, $2)
		 ON CONFLICT DO NOTHING`,
		fmt.Sprintf("role:%d", DefaultRoleID), contracts.PermWithdraw.Key())
	return err
}

func downSeedDefaultGrant(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx,
		`DELETE FROM authz_svc.casbin_rule WHERE ptype = 'p' AND v1 LIKE 'wallet:%'`)
	return err
}
```

`embed.go` is two lines: `//go:embed *.sql` into an `embed.FS`, exposed as
`fs.FS`. Boot refuses to start if the database grants a permission no
enabled module declares, so removing a module means dropping its grants.

## 10. Ports and adapters: the extraction seam

The consumer declares what it needs. It never imports the provider's
service, only its `contracts`.

```go
// internal/ports/ports.go
type UserReader interface {
	ByID(ctx context.Context, id string) (UserSnapshot, error)
	ByIDs(ctx context.Context, ids []string) (map[string]UserSnapshot, error)
}

// UserSnapshot is wallet's own projection of a user — never user's entity.
type UserSnapshot struct {
	ID       string
	Status   string
	Currency string
}

func (s UserSnapshot) Active() bool { return s.Status == "active" }
```

```go
// ports.go (module root): aliases so the registry can name the port.
type UserReader = ports.UserReader
type UserSnapshot = ports.UserSnapshot
```

Ship the batch method before it is needed. Its absence is what turns a JOIN
into fifty HTTP calls on extraction day.

**Local adapter**, in-process:

```go
// adapters/user_local.go
type LocalUserReader struct {
	users usercontracts.Reader
}

func NewLocalUserReader(users usercontracts.Reader) *LocalUserReader {
	return &LocalUserReader{users: users}
}

func (l *LocalUserReader) ByID(ctx context.Context, id string) (ports.UserSnapshot, error) {
	u, err := l.users.Snapshot(ctx, id)
	if err != nil {
		return ports.UserSnapshot{}, err
	}
	return toSnapshot(u), nil
}

func (l *LocalUserReader) ByIDs(ctx context.Context, ids []string) (map[string]ports.UserSnapshot, error) {
	users, err := l.users.SnapshotByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ports.UserSnapshot, len(users))
	for id, u := range users {
		out[id] = toSnapshot(u)
	}
	return out, nil
}

func toSnapshot(u usercontracts.User) ports.UserSnapshot {
	return ports.UserSnapshot{ID: string(u.ID), Status: string(u.Status), Currency: u.Currency}
}
```

**Remote adapter**, the same port over HTTP with timeout, breaker, batch:

```go
// adapters/user_remote.go
type RemoteUserReader struct {
	base    string
	client  *http.Client
	breaker *gobreaker.CircuitBreaker[map[string]ports.UserSnapshot]
}

// The breaker opens after 5 consecutive failures and probes again after
// 10s: a dead user service must cost microseconds, not a timeout per call.
func NewRemoteUserReader(baseURL string, timeout time.Duration) *RemoteUserReader {
	return &RemoteUserReader{
		base:   strings.TrimRight(baseURL, "/"),
		client: &http.Client{Timeout: timeout},
		breaker: gobreaker.NewCircuitBreaker[map[string]ports.UserSnapshot](gobreaker.Settings{
			Name: "wallet→user",
			ReadyToTrip: func(c gobreaker.Counts) bool {
				return c.ConsecutiveFailures >= 5
			},
			Timeout: 10 * time.Second,
		}),
	}
}

func (r *RemoteUserReader) ByID(ctx context.Context, id string) (ports.UserSnapshot, error) {
	got, err := r.ByIDs(ctx, []string{id})
	if err != nil {
		return ports.UserSnapshot{}, err
	}
	snap, ok := got[id]
	if !ok {
		return ports.UserSnapshot{}, errs.New(errs.NotFound, "user not found")
	}
	return snap, nil
}

// ByIDs is ONE round trip for any batch size.
func (r *RemoteUserReader) ByIDs(ctx context.Context, ids []string) (map[string]ports.UserSnapshot, error) {
	if len(ids) == 0 {
		return map[string]ports.UserSnapshot{}, nil
	}
	out, err := r.breaker.Execute(func() (map[string]ports.UserSnapshot, error) {
		return r.fetch(ctx, ids)
	})
	if err != nil {
		if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
			return nil, errs.Wrap(errs.Unavailable, "user service circuit open", err)
		}
		return nil, err
	}
	return out, nil
}

// batchResponse mirrors user's internal batch endpoint shape. Duplicated by
// design: importing user's transport DTOs would couple the modules — this
// adapter depends on the WIRE contract, not on user's code.
type batchResponse struct {
	Users map[string]struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Currency string `json:"currency"`
	} `json:"users"`
}

func (r *RemoteUserReader) fetch(ctx context.Context, ids []string) (map[string]ports.UserSnapshot, error) {
	u := fmt.Sprintf("%s/internal/users?ids=%s", r.base, url.QueryEscape(strings.Join(ids, ",")))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "build user request", err)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, errs.Wrap(errs.Unavailable, "user service", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return map[string]ports.UserSnapshot{}, nil
	case resp.StatusCode != http.StatusOK:
		return nil, errs.New(errs.Unavailable, fmt.Sprintf("user service returned %d", resp.StatusCode))
	}

	var body batchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, errs.Wrap(errs.Internal, "decode user response", err)
	}
	out := make(map[string]ports.UserSnapshot, len(body.Users))
	for id, u := range body.Users {
		out[id] = ports.UserSnapshot{ID: u.ID, Status: u.Status, Currency: u.Currency}
	}
	return out, nil
}
```

Add the port to `.mockery.yaml`. Mocks are generated for ports only, never
for a module's own repository.

## 11. Subscribing: idempotent by construction

Wallet's handler is in section 2. Notification's records a row keyed by the
event id, so a redelivery changes nothing.

```go
func (m *Module) onUserRegistered(ctx context.Context, e bus.Envelope) error {
	var ev usercontracts.UserRegisteredV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		// errs.Invalid → the consumer loop dead-letters instead of
		// retrying: the payload will be exactly as invalid in 5 seconds.
		return errs.Wrap(errs.Invalid, "undecodable user.registered.v1", err)
	}
	return m.svc.CreateFromEvent(ctx, e.EventID, "user.registered", "welcome "+ev.Email)
}

func (s *Service) CreateFromEvent(ctx context.Context, eventID, kind, body string) error {
	if eventID == "" {
		return errs.New(errs.Invalid, "event id required")
	}
	return s.repo.upsert(ctx, notification{
		ID:        eventID,
		Kind:      kind,
		Body:      body,
		CreatedAt: s.clock.Now(),
	})
}

// upsert is INSERT ... ON CONFLICT DO NOTHING.
func (r *gormRepo) upsert(ctx context.Context, n notification) error {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&n).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "upsert notification", err)
	}
	return nil
}
```

Delivery is at least once and unordered. Redis dedupe on `event_id` is the
belt; the upsert is the braces. Any error other than `Invalid` climbs the
retry ladder and then parks in the DLQ and the dead-letter table.

## 12. Auth flow

The user module owned credentials, so it alone received `*authn.Auth`.
Login and lookup failures return the same error, so email existence does
not leak.

```go
var ErrBadCredentials = errs.New(errs.Unauthenticated, "invalid credentials")

type TokenPair struct {
	Access  string
	Refresh string
	User    domain.User
}

func (s *Service) Login(ctx context.Context, email, password string) (TokenPair, error) {
	if s.auth == nil {
		return TokenPair{}, errs.New(errs.Internal, "auth not wired")
	}
	u, err := s.repo.ByEmail(ctx, email)
	if err != nil {
		if errs.KindOf(err) == errs.NotFound {
			return TokenPair{}, ErrBadCredentials
		}
		return TokenPair{}, err
	}
	if !u.Active() || u.PasswordHash == "" {
		return TokenPair{}, ErrBadCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return TokenPair{}, ErrBadCredentials
	}

	access, _, err := s.auth.Issuer.IssueAccess(string(u.ID), s.roles())
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := s.auth.Refresh.Issue(ctx, string(u.ID))
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{Access: access, Refresh: refresh, User: u}, nil
}

// Refresh rotates the presented refresh token (replay revokes the family)
// and mints a fresh access token.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	next, err := s.auth.Refresh.Rotate(ctx, refreshToken)
	if err != nil {
		return TokenPair{}, err
	}
	userID, _, err := authn.DecodeUserID(next)
	if err != nil {
		return TokenPair{}, err
	}
	u, err := s.repo.ByID(ctx, userID)
	if err != nil {
		return TokenPair{}, err
	}
	access, _, err := s.auth.Issuer.IssueAccess(userID, s.roles())
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{Access: access, Refresh: next, User: u}, nil
}

// Logout denies the presented access token for its remaining TTL and
// revokes every refresh token (logout-everywhere).
func (s *Service) Logout(ctx context.Context, p authz.Principal) error {
	if jti, ok := authn.JTIFrom(ctx); ok {
		_ = s.auth.Refresh.Deny(ctx, jti, s.auth.Issuer.AccessTTL())
	}
	return s.auth.Refresh.RevokeAll(ctx, p.UserID)
}

func hashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errs.New(errs.Invalid, "password must be at least 8 characters")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", errs.Wrap(errs.Internal, "hash password", err)
	}
	return string(h), nil
}

// roles is what an authenticated principal carries. Every user holds the
// default role until a role-administration surface exists; the
// service-layer ownership checks are what actually protect resources.
func (s *Service) roles() []int64 {
	if s.defaultRoleID == 0 {
		return nil
	}
	return []int64{s.defaultRoleID}
}
```

The module config carried `DEFAULT_ROLE_ID` (default 1) and a `CACHE_TTL`.

## 13. Cached repository decorator

Read-through over the GORM repository. Writes pass through. Eviction is
called by the service after commit, never inside the transaction. The `v1`
key segment is the invalidation escape hatch: changing the cached shape is
a version bump, not a flush.

```go
type Reads interface {
	ByID(ctx context.Context, id string) (domain.User, error)
	ByIDs(ctx context.Context, ids []string) (map[string]domain.User, error)
	ByEmail(ctx context.Context, email string) (domain.User, error)
}

type Cached struct {
	*Postgres
	reads Reads
	cache *redis.ModuleCache
}

const keyVersion = "user:v1:"

func NewCached(inner *Postgres, cache *redis.ModuleCache) *Cached {
	return &Cached{Postgres: inner, reads: inner, cache: cache}
}

func (c *Cached) ByID(ctx context.Context, id string) (domain.User, error) {
	var u domain.User
	err := c.cache.GetOrLoad(ctx, keyVersion+id, &u, func(ctx context.Context) (any, error) {
		return c.reads.ByID(ctx, id)
	})
	return u, err
}

func (c *Cached) ByIDs(ctx context.Context, ids []string) (map[string]domain.User, error) {
	keyed := make([]string, len(ids))
	for i, id := range ids {
		keyed[i] = keyVersion + id
	}
	got, err := redis.MGetOrLoad(ctx, c.cache, keyed,
		func(ctx context.Context, missing []string) (map[string]domain.User, error) {
			raw := make([]string, len(missing))
			for i, k := range missing {
				raw[i] = k[len(keyVersion):]
			}
			loaded, err := c.reads.ByIDs(ctx, raw)
			if err != nil {
				return nil, err
			}
			out := make(map[string]domain.User, len(loaded))
			for id, u := range loaded {
				out[keyVersion+id] = u
			}
			return out, nil
		})
	if err != nil {
		return nil, err
	}
	out := make(map[string]domain.User, len(got))
	for k, u := range got {
		out[k[len(keyVersion):]] = u
	}
	return out, nil
}

// ByEmail stays uncached: it serves login, which wants the freshest row.
func (c *Cached) ByEmail(ctx context.Context, email string) (domain.User, error) {
	return c.reads.ByEmail(ctx, email)
}

// Evict drops cache entries. Call after commit only.
func (c *Cached) Evict(ctx context.Context, ids ...string) {
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = keyVersion + id
	}
	_ = c.cache.Del(ctx, keys...)
}
```

Wired in `module.go`:

```go
r := repo.NewCached(repo.NewPostgres(d.DB), d.Cache.WithTTL(cfg.CacheTTL))
```

## 14. The thin shape

Notification collapsed the layers. The GORM model doubles as the module's
type, which is acceptable exactly because it stays unexported and never
crosses into `contracts/`. Keyset pagination by `(created_at, id)`:

```go
type notification struct {
	ID        string    `gorm:"primaryKey;type:uuid"`
	Kind      string    `gorm:"not null"`
	Body      string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null;index:idx_notifications_created,sort:desc"`
}

func (r *gormRepo) list(ctx context.Context, before time.Time, beforeID string, limit int) ([]notification, error) {
	q := r.db.WithContext(ctx).Order("created_at DESC, id DESC").Limit(limit)
	if !before.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", before, beforeID)
	}
	var out []notification
	if err := q.Find(&out).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list notifications", err)
	}
	return out, nil
}

// List pages newest-first by (created_at, id) keyset cursor.
func (s *Service) List(ctx context.Context, cursor string, limit int) ([]Notification, error) {
	if limit <= 0 || limit > maxPageSize {
		limit = maxPageSize
	}
	var before time.Time
	var beforeID string
	if cursor != "" {
		var err error
		before, beforeID, err = pagination.DecodeCursor(cursor)
		if err != nil {
			return nil, err
		}
	}
	rows, err := s.repo.list(ctx, before, beforeID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Notification, len(rows))
	for i, r := range rows {
		out[i] = fromModel(r)
	}
	return out, nil
}

func NextCursor(last Notification) string {
	return pagination.EncodeCursor(last.CreatedAt, last.ID)
}
```

## 15. Tests

**Service unit tests** use a hand-written fake repository, a fake outbox, a
fake clock, a permission allow-list, and a pass-through transaction runner.
No database.

```go
type fakeOutbox struct{ published []recordedEvent }

func (f *fakeOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, payload any) error {
	f.published = append(f.published, recordedEvent{topic, payload})
	return nil
}

// allowKeys enforces only the listed permission keys — everything else is
// denied, so the tests can hand a principal a role-level grant and still
// prove the SERVICE refuses foreign wallets.
type allowKeys map[string]bool

func (a allowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}

func newTestService(repo Repository, users ports.UserReader, ob *fakeOutbox, enf authz.Enforcer) *Service {
	svc := NewService(repo, users, ob, enf, nil, clock.NewFake(time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)))
	svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) }
	return svc
}

// Role-level permission passes, the SERVICE still refuses a wallet the
// principal does not own — middleware cannot make this call.
func TestWithdrawFromForeignWalletDeniedDespiteRole(t *testing.T) {
	repo, ob := newFakeRepo(), &fakeOutbox{}
	w := seedWallet(t, repo, "owner", 10_000)
	svc := newTestService(repo, &fakeUsers{}, ob, allowKeys{"wallet:withdraw": true})

	thief := authz.Principal{UserID: "someone-else"} // holds wallet:withdraw!
	_, err := svc.Withdraw(context.Background(), thief, string(w.ID), 100)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Empty(t, ob.published, "denied withdrawal must not emit an event")
}

func TestCreateForUserIsIdempotent(t *testing.T) {
	repo, ob := newFakeRepo(), &fakeOutbox{}
	users := &fakeUsers{users: map[string]ports.UserSnapshot{"u1": activeUser("u1")}}
	svc := newTestService(repo, users, ob, allowKeys{})

	first, created, err := svc.CreateForUser(context.Background(), "u1", "USD")
	require.NoError(t, err)
	require.True(t, created)

	again, created, err := svc.CreateForUser(context.Background(), "u1", "USD")
	require.NoError(t, err)
	require.False(t, created, "second delivery of user.registered must be a no-op")
	require.Equal(t, first.ID, again.ID, "idempotent create must return the existing wallet")
}

// The mockery-generated port mock: the contract is what must survive
// extraction.
func TestPortContractWithGeneratedMock(t *testing.T) {
	users := mocks.NewMockUserReader(t)
	users.EXPECT().
		ByID(mock.Anything, "u-mocked").
		Return(ports.UserSnapshot{ID: "u-mocked", Status: "active", Currency: "USD"}, nil).
		Once()

	repo, ob := newFakeRepo(), &fakeOutbox{}
	svc := newTestService(repo, users, ob, allowKeys{})

	_, created, err := svc.CreateForUser(context.Background(), "u-mocked", "USD")
	require.NoError(t, err)
	require.True(t, created)
}
```

**Repository tests** run against testcontainers Postgres, never mocks.

```go
func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "wallet", migrations.FS))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "wallet")
	return NewPostgres(moduleDB), moduleDB
}

func TestSaveDetectsVersionConflict(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()

	w := mustWallet(t, "0198d000-0000-7000-8000-000000000002")
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		_, err := r.Create(ctx, tx, w)
		return err
	}))

	require.NoError(t, w.Deposit(100, time.Now().UTC()))
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		return r.Save(ctx, tx, w)
	}))

	// A save from the same stale snapshot must be refused.
	err := postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		return r.Save(ctx, tx, w)
	})
	require.ErrorIs(t, err, domain.ErrVersionConflict)
}
```

**Remote adapter tests** use `httptest` and prove the timeout and breaker.

```go
func TestRemoteBreakerOpensAndShortCircuits(t *testing.T) {
	var hits atomic.Int64
	srv := stubUserService(t, func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})

	r := NewRemoteUserReader(srv.URL, time.Second)
	ctx := context.Background()
	for range 5 {
		_, err := r.ByIDs(ctx, []string{"u1"})
		require.Error(t, err)
	}
	require.EqualValues(t, 5, hits.Load())

	// Breaker is now open: calls fail fast without touching the wire.
	_, err := r.ByIDs(ctx, []string{"u1"})
	require.Equal(t, errs.Unavailable, errs.KindOf(err))
	require.Contains(t, err.Error(), "circuit open")
	require.EqualValues(t, 5, hits.Load(), "an open breaker must not hit the backend")
}
```

**The e2e journey** drove the real binaries over HTTP only: register, log
in, refresh with rotation and replay rejection, poll until the wallet
appeared without anyone calling `POST /wallets`, deposit twice with one
`Idempotency-Key` and get charged once, withdraw, overdraw and get 409, and
confirm another user gets 403 on the wallet and an anonymous caller 401.
The harness that boots the binaries is still in `test/e2e`.

## 16. Multi-step workflows

Nothing above spans two modules in one transaction, and nothing should.
For sagas, reservations with a TTL, external providers with idempotency
keys, and reconcile sweeps for stuck rows, follow the rules in `CLAUDE.md`
under "Multi-step workflows across modules". The event path is the default.
