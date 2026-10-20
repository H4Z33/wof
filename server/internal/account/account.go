// Package account stores accounts and characters in PostgreSQL.
package account

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"convergence/server/internal/model"
)

const MaxCharactersPerAccount = 4

var (
	ErrUsernameTaken     = errors.New("username taken")
	ErrBadCredentials    = errors.New("wrong username or password")
	ErrNameTaken         = errors.New("character name taken")
	ErrTooManyCharacters = errors.New("character limit reached")
	ErrNotFound          = errors.New("not found")
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *Service) Register(ctx context.Context, username, password string) error {
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO accounts (username, password_hash) VALUES ($1, $2)`,
		strings.ToLower(username), hash)
	if isUniqueViolation(err) {
		return ErrUsernameTaken
	}
	return err
}

var (
	dummyHashOnce sync.Once
	dummyHash     string
)

// Authenticate returns the account ID for valid credentials.
func (s *Service) Authenticate(ctx context.Context, username, password string) (int64, error) {
	var id int64
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT id, password_hash FROM accounts WHERE username = $1`,
		strings.ToLower(username)).Scan(&id, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		// Hash anyway so response time does not reveal which usernames exist.
		dummyHashOnce.Do(func() { dummyHash, _ = hashPassword("dummy-password") })
		verifyPassword(password, dummyHash)
		return 0, ErrBadCredentials
	}
	if err != nil {
		return 0, err
	}
	ok, err := verifyPassword(password, hash)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrBadCredentials
	}
	return id, nil
}

const characterColumns = `id, account_id, name, model, class, zone_id, pos_x, pos_y, pos_z, yaw, level, exp, hp, luck, karma, inventory, progression, renown, title, quests`

func scanCharacter(row pgx.Row) (model.Character, error) {
	var c model.Character
	err := row.Scan(&c.ID, &c.AccountID, &c.Name, &c.Model, &c.Class, &c.ZoneID,
		&c.Pos[0], &c.Pos[1], &c.Pos[2], &c.Yaw, &c.Level, &c.Exp, &c.HP, &c.Luck, &c.Karma, &c.Inventory, &c.Progression, &c.Renown, &c.Title, &c.Quests)
	return c, err
}

func (s *Service) Characters(ctx context.Context, accountID int64) ([]model.Character, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+characterColumns+` FROM characters WHERE account_id = $1 ORDER BY id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Character
	for rows.Next() {
		c, err := scanCharacter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Character loads one character, checking that it belongs to the account.
func (s *Service) Character(ctx context.Context, accountID, characterID int64) (model.Character, error) {
	c, err := scanCharacter(s.pool.QueryRow(ctx,
		`SELECT `+characterColumns+` FROM characters WHERE id = $1 AND account_id = $2`, characterID, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// CreateCharacter inserts c (ID is ignored), enforcing the per-account limit.
func (s *Service) CreateCharacter(ctx context.Context, c model.Character) (model.Character, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return c, err
	}
	defer tx.Rollback(ctx)

	// Lock the account row so concurrent creates cannot exceed the limit.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM accounts WHERE id = $1 FOR UPDATE`, c.AccountID); err != nil {
		return c, err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM characters WHERE account_id = $1`, c.AccountID).Scan(&count); err != nil {
		return c, err
	}
	if count >= MaxCharactersPerAccount {
		return c, ErrTooManyCharacters
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO characters (account_id, name, model, class, zone_id, pos_x, pos_y, pos_z, yaw, level, exp, hp, luck, karma, inventory)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) RETURNING id`,
		c.AccountID, c.Name, c.Model, c.Class, c.ZoneID, c.Pos[0], c.Pos[1], c.Pos[2], c.Yaw, c.Level, c.Exp, c.HP,
		c.Luck, c.Karma, inventoryJSON(c.Inventory),
	).Scan(&c.ID)
	if isUniqueViolation(err) {
		return c, ErrNameTaken
	}
	if err != nil {
		return c, err
	}
	return c, tx.Commit(ctx)
}

// questsJSON never returns nil, so the column is '{}' rather than 'null'.
func questsJSON(q map[string]model.QuestState) map[string]model.QuestState {
	if q == nil {
		return map[string]model.QuestState{}
	}
	return q
}

// inventoryJSON never returns nil, so the column is '{}' rather than 'null'.
func inventoryJSON(inv map[string]int) map[string]int {
	if inv == nil {
		return map[string]int{}
	}
	return inv
}

// SaveCharacters writes the mutable state of each character in one batch.
func (s *Service) SaveCharacters(ctx context.Context, chars []model.Character) error {
	batch := &pgx.Batch{}
	for _, c := range chars {
		batch.Queue(`UPDATE characters
			SET zone_id = $2, pos_x = $3, pos_y = $4, pos_z = $5, yaw = $6, level = $7, exp = $8, hp = $9,
			    karma = $10, inventory = $11, progression = $12, renown = $13, title = $14, quests = $15, updated_at = now()
			WHERE id = $1`,
			c.ID, c.ZoneID, c.Pos[0], c.Pos[1], c.Pos[2], c.Yaw, c.Level, c.Exp, c.HP, c.Karma, inventoryJSON(c.Inventory), c.Progression,
			c.Renown, c.Title, questsJSON(c.Quests))
	}
	return s.pool.SendBatch(ctx, batch).Close()
}
