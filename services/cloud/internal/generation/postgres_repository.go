package generation

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

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
	session, err := repository.updateInTransaction(ctx, transaction, sessionID, userID, change)
	if err != nil {
		return nil, err
	}
	if err := transaction.Commit(); err != nil {
		return nil, err
	}
	return session, nil
}

func (repository *PostgresRepository) ConfirmAndEnqueue(
	ctx context.Context,
	userID, sessionID string,
	change func(*Session) error,
	jobID string,
	at time.Time,
) (*Session, error) {
	transaction, err := repository.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = transaction.Rollback() }()
	session, err := repository.updateInTransaction(ctx, transaction, sessionID, userID, change)
	if err != nil {
		return nil, err
	}
	_, err = transaction.ExecContext(
		ctx,
		`INSERT INTO generation_jobs (
		  id, session_id, status, attempts, max_attempts,
		  available_at, created_at, updated_at
		) VALUES ($1, $2, 'queued', 0, 3, $3, $3, $3)`,
		jobID, sessionID, at.UTC(),
	)
	if err != nil {
		return nil, err
	}
	if err := transaction.Commit(); err != nil {
		return nil, err
	}
	return session, nil
}

func (repository *PostgresRepository) updateInTransaction(
	ctx context.Context,
	transaction *sql.Tx,
	sessionID, userID string,
	change func(*Session) error,
) (*Session, error) {
	session, err := loadSession(ctx, transaction, sessionID, userID, true)
	if err != nil {
		return nil, err
	}
	previousStatus := session.Status
	previousConfirmedRequirement, err := encodeConfirmedRequirement(session.ConfirmedRequirement)
	if err != nil {
		return nil, err
	}
	messageCount := len(session.Messages)
	eventCount := len(session.Events)
	if err := change(session); err != nil {
		return nil, err
	}
	if err := enforceConfirmedRequirementWriteOnce(
		previousStatus,
		previousConfirmedRequirement,
		session,
	); err != nil {
		return nil, err
	}
	summary, err := json.Marshal(session.Summary)
	if err != nil {
		return nil, err
	}
	confirmedRequirement, err := encodeConfirmedRequirement(session.ConfirmedRequirement)
	if err != nil {
		return nil, err
	}
	result, err := transaction.ExecContext(
		ctx,
		`UPDATE generation_sessions
		 SET status = $1, summary_json = $2, confirmed_requirement_json = $3,
		     version_id = NULLIF($4, ''), updated_at = $5
		 WHERE id = $6`,
		session.Status,
		summary,
		confirmedRequirement,
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
	confirmedRequirement, err := encodeConfirmedRequirement(session.ConfirmedRequirement)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(
		ctx,
		`INSERT INTO generation_sessions (
		  id, user_id, prompt, target, locale, status, summary_json,
		  confirmed_requirement_json, version_id, base_card_id, base_version_id,
		  created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''),
		          NULLIF($10, ''), NULLIF($11, ''), $12, $13)`,
		session.ID,
		session.UserID,
		session.Prompt,
		session.Target,
		session.Locale,
		session.Status,
		summary,
		confirmedRequirement,
		session.VersionID,
		session.BaseCardID,
		session.BaseVersionID,
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
	                 summary_json, confirmed_requirement_json,
	                 version_id, base_card_id, base_version_id,
	                 created_at, updated_at
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
	var confirmedRequirement []byte
	var versionID sql.NullString
	var baseCardID sql.NullString
	var baseVersionID sql.NullString
	if err := executor.QueryRowContext(ctx, query, arguments...).Scan(
		&session.ID,
		&session.UserID,
		&session.Prompt,
		&session.Target,
		&session.Locale,
		&session.Status,
		&summary,
		&confirmedRequirement,
		&versionID,
		&baseCardID,
		&baseVersionID,
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
	if confirmedRequirement != nil {
		requirement, err := decodeConfirmedRequirement(confirmedRequirement)
		if err != nil {
			return nil, fmt.Errorf("decode generation confirmed requirement: %w", err)
		}
		session.ConfirmedRequirement = requirement
	}
	session.VersionID = versionID.String
	session.BaseCardID = baseCardID.String
	session.BaseVersionID = baseVersionID.String
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

func encodeConfirmedRequirement(requirement *RequirementSnapshot) ([]byte, error) {
	if requirement == nil {
		return nil, nil
	}
	if err := validateConfirmedRequirement(requirement); err != nil {
		return nil, fmt.Errorf("encode generation confirmed requirement: %w", err)
	}
	encoded, err := json.Marshal(requirement)
	if err != nil {
		return nil, fmt.Errorf("encode generation confirmed requirement: %w", err)
	}
	return encoded, nil
}

func enforceConfirmedRequirementWriteOnce(
	previousStatus Status,
	previousRequirement []byte,
	session *Session,
) error {
	if previousRequirement == nil {
		if session.ConfirmedRequirement != nil &&
			(previousStatus != StatusAwaitingConfirmation || session.Status != StatusQueued) {
			return fmt.Errorf("%w: confirmed requirement can only be set while confirming", ErrConflict)
		}
		return nil
	}
	if session.ConfirmedRequirement == nil {
		return fmt.Errorf("%w: confirmed requirement is immutable", ErrConflict)
	}
	currentRequirement, err := json.Marshal(session.ConfirmedRequirement)
	if err != nil || !bytes.Equal(currentRequirement, previousRequirement) {
		return fmt.Errorf("%w: confirmed requirement is immutable", ErrConflict)
	}
	return nil
}

func decodeConfirmedRequirement(encoded []byte) (*RequirementSnapshot, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var requirement *RequirementSnapshot
	if err := decoder.Decode(&requirement); err != nil {
		return nil, err
	}
	if requirement == nil {
		return nil, fmt.Errorf("value is null")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	if err := validateConfirmedRequirement(requirement); err != nil {
		return nil, err
	}
	return requirement, nil
}

func validateConfirmedRequirement(requirement *RequirementSnapshot) error {
	if requirement == nil {
		return fmt.Errorf("value is null")
	}
	if strings.TrimSpace(requirement.InitialPrompt) == "" {
		return fmt.Errorf("initialPrompt is required")
	}
	if requirement.AdditionalMessages == nil {
		return fmt.Errorf("additionalMessages is required")
	}
	for index, message := range requirement.AdditionalMessages {
		if message.Role != "user" {
			return fmt.Errorf("additionalMessages[%d].role must be user", index)
		}
		if strings.TrimSpace(message.Content) == "" {
			return fmt.Errorf("additionalMessages[%d].content is required", index)
		}
		if message.CreatedAt.IsZero() {
			return fmt.Errorf("additionalMessages[%d].createdAt is required", index)
		}
	}
	switch requirement.Target {
	case TargetAuto, TargetNative, TargetWeb:
	default:
		return fmt.Errorf("target is invalid")
	}
	if strings.TrimSpace(requirement.Locale) == "" {
		return fmt.Errorf("locale is required")
	}
	if (strings.TrimSpace(requirement.BaseCardID) == "") !=
		(strings.TrimSpace(requirement.BaseVersionID) == "") {
		return fmt.Errorf("baseCardId and baseVersionId must be provided together")
	}
	if requirement.AllowedCapabilities == nil {
		return fmt.Errorf("allowedCapabilities is required")
	}
	seenCapabilities := make(map[string]struct{}, len(requirement.AllowedCapabilities))
	for index, capability := range requirement.AllowedCapabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			return fmt.Errorf("allowedCapabilities[%d] is required", index)
		}
		if _, exists := seenCapabilities[capability]; exists {
			return fmt.Errorf("allowedCapabilities[%d] is duplicated", index)
		}
		seenCapabilities[capability] = struct{}{}
	}
	if requirement.ConfirmedAt.IsZero() {
		return fmt.Errorf("confirmedAt is required")
	}
	return nil
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
