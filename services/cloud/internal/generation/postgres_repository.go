package generation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct {
	database *sql.DB
}

func NewPostgresRepository(database *sql.DB) *PostgresRepository {
	return &PostgresRepository{database: database}
}

func (*PostgresRepository) RequiresExternalEventPolling() bool {
	return true
}

func (repository *PostgresRepository) Create(ctx context.Context, session *Session) error {
	transaction, err := repository.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	if err := insertSession(ctx, transaction, session); err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	if err := insertMessages(ctx, transaction, session.ID, session.Messages, 0); err != nil {
		return err
	}
	if err := insertEvents(ctx, transaction, session.Events, 0); err != nil {
		return err
	}
	return transaction.Commit()
}

func (repository *PostgresRepository) Get(
	ctx context.Context,
	userID string,
	sessionID string,
) (*Session, error) {
	return loadSession(ctx, repository.database, sessionID, userID, false)
}

func (repository *PostgresRepository) GetSystem(
	ctx context.Context,
	sessionID string,
) (*Session, error) {
	return loadSession(ctx, repository.database, sessionID, "", false)
}

func (repository *PostgresRepository) Update(
	ctx context.Context,
	userID string,
	sessionID string,
	change func(*Session) error,
) (*Session, error) {
	return repository.update(ctx, sessionID, userID, change)
}

func (repository *PostgresRepository) UpdateSystem(
	ctx context.Context,
	sessionID string,
	change func(*Session) error,
) (*Session, error) {
	return repository.update(ctx, sessionID, "", change)
}

func (repository *PostgresRepository) update(
	ctx context.Context,
	sessionID string,
	userID string,
	change func(*Session) error,
) (*Session, error) {
	transaction, err := repository.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = transaction.Rollback() }()
	session, err := loadSession(ctx, transaction, sessionID, userID, true)
	if err != nil {
		return nil, err
	}
	messageCount := len(session.Messages)
	eventCount := len(session.Events)
	if err := change(session); err != nil {
		return nil, err
	}
	summary, err := json.Marshal(session.Summary)
	if err != nil {
		return nil, err
	}
	result, err := transaction.ExecContext(
		ctx,
		`UPDATE generation_sessions
		 SET status = $1, summary_json = $2, version_id = NULLIF($3, ''), updated_at = $4
		 WHERE id = $5`,
		session.Status,
		summary,
		session.VersionID,
		session.UpdatedAt.UTC(),
		session.ID,
	)
	if err != nil {
		return nil, err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return nil, err
		}
		return nil, ErrNotFound
	}
	if err := insertMessages(ctx, transaction, session.ID, session.Messages, messageCount); err != nil {
		return nil, err
	}
	if err := insertEvents(ctx, transaction, session.Events, eventCount); err != nil {
		return nil, err
	}
	if err := transaction.Commit(); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func insertSession(ctx context.Context, executor sqlExecutor, session *Session) error {
	summary, err := json.Marshal(session.Summary)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(
		ctx,
		`INSERT INTO generation_sessions (
		  id, user_id, prompt, target, locale, status, summary_json,
		  version_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, $10)`,
		session.ID,
		session.UserID,
		session.Prompt,
		session.Target,
		session.Locale,
		session.Status,
		summary,
		session.VersionID,
		session.CreatedAt.UTC(),
		session.UpdatedAt.UTC(),
	)
	return err
}

func insertMessages(
	ctx context.Context,
	executor sqlExecutor,
	sessionID string,
	messages []Message,
	start int,
) error {
	for index := start; index < len(messages); index++ {
		message := messages[index]
		if _, err := executor.ExecContext(
			ctx,
			`INSERT INTO generation_messages
			 (session_id, sequence, role, content, created_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			sessionID,
			index+1,
			message.Role,
			message.Content,
			message.CreatedAt.UTC(),
		); err != nil {
			return err
		}
	}
	return nil
}

func insertEvents(
	ctx context.Context,
	executor sqlExecutor,
	events []Event,
	start int,
) error {
	for index := start; index < len(events); index++ {
		event := events[index]
		if _, err := executor.ExecContext(
			ctx,
			`INSERT INTO generation_events (
			  session_id, event_id, type, stage, message, progress,
			  version_id, error_code, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), $9)`,
			event.SessionID,
			event.EventID,
			event.Type,
			event.Stage,
			event.Message,
			event.Progress,
			event.VersionID,
			event.ErrorCode,
			event.Timestamp.UTC(),
		); err != nil {
			return err
		}
	}
	return nil
}

func loadSession(
	ctx context.Context,
	executor sqlExecutor,
	sessionID string,
	userID string,
	forUpdate bool,
) (*Session, error) {
	query := `SELECT id, user_id, prompt, target, locale, status,
	                 summary_json, version_id, created_at, updated_at
	          FROM generation_sessions WHERE id = $1`
	arguments := []any{sessionID}
	if userID != "" {
		query += ` AND user_id = $2`
		arguments = append(arguments, userID)
	}
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var session Session
	var summary []byte
	var versionID sql.NullString
	if err := executor.QueryRowContext(ctx, query, arguments...).Scan(
		&session.ID,
		&session.UserID,
		&session.Prompt,
		&session.Target,
		&session.Locale,
		&session.Status,
		&summary,
		&versionID,
		&session.CreatedAt,
		&session.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal(summary, &session.Summary); err != nil {
		return nil, fmt.Errorf("decode generation summary: %w", err)
	}
	session.VersionID = versionID.String
	messages, err := loadMessages(ctx, executor, session.ID)
	if err != nil {
		return nil, err
	}
	events, err := loadEvents(ctx, executor, session.ID)
	if err != nil {
		return nil, err
	}
	session.Messages = messages
	session.Events = events
	if len(events) > 0 {
		session.nextEventID = events[len(events)-1].EventID
	}
	return &session, nil
}

func loadMessages(ctx context.Context, executor sqlExecutor, sessionID string) ([]Message, error) {
	rows, err := executor.QueryContext(
		ctx,
		`SELECT role, content, created_at FROM generation_messages
		 WHERE session_id = $1 ORDER BY sequence`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]Message, 0)
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.Role, &message.Content, &message.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func loadEvents(ctx context.Context, executor sqlExecutor, sessionID string) ([]Event, error) {
	rows, err := executor.QueryContext(
		ctx,
		`SELECT event_id, type, stage, message, progress,
		        version_id, error_code, created_at
		 FROM generation_events WHERE session_id = $1 ORDER BY event_id`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		var versionID, errorCode sql.NullString
		if err := rows.Scan(
			&event.EventID,
			&event.Type,
			&event.Stage,
			&event.Message,
			&event.Progress,
			&versionID,
			&errorCode,
			&event.Timestamp,
		); err != nil {
			return nil, err
		}
		event.SessionID = sessionID
		event.VersionID = versionID.String
		event.ErrorCode = errorCode.String
		events = append(events, event)
	}
	return events, rows.Err()
}

func isUniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}
