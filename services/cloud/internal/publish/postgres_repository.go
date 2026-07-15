package publish

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type PostgresVersionRepository struct {
	database *sql.DB
}

func NewPostgresVersionRepository(database *sql.DB) *PostgresVersionRepository {
	return &PostgresVersionRepository{database: database}
}

func (repository *PostgresVersionRepository) Create(
	ctx context.Context,
	version CardVersion,
) (CardVersion, error) {
	preview, err := json.Marshal(version.Preview)
	if err != nil {
		return CardVersion{}, err
	}
	result, err := repository.database.ExecContext(
		ctx,
		`INSERT INTO card_versions (
		  version_id, card_id, user_id, runtime, display_version,
		  title, description, artifact_object_key, artifact_sha256,
		  key_id, preview_json, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT DO NOTHING`,
		version.VersionID,
		version.CardID,
		version.UserID,
		version.Runtime,
		version.DisplayVersion,
		version.Title,
		version.Description,
		version.ArtifactKey,
		version.ArtifactSHA256,
		version.KeyID,
		preview,
		version.CreatedAt.UTC(),
	)
	if err != nil {
		return CardVersion{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return CardVersion{}, err
	}
	if affected == 1 {
		return cloneVersion(version), nil
	}
	existing, err := repository.Find(ctx, version.UserID, version.CardID, version.VersionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			var displayExists bool
			if queryErr := repository.database.QueryRowContext(
				ctx,
				`SELECT EXISTS (
				  SELECT 1 FROM card_versions
				  WHERE user_id = $1 AND card_id = $2 AND display_version = $3
				)`,
				version.UserID, version.CardID, version.DisplayVersion,
			).Scan(&displayExists); queryErr != nil {
				return CardVersion{}, queryErr
			}
			if displayExists {
				return CardVersion{}, ErrDisplayVersionConflict
			}
			return CardVersion{}, ErrVersionConflict
		}
		return CardVersion{}, err
	}
	if existing.ArtifactSHA256 != version.ArtifactSHA256 ||
		existing.CardID != version.CardID ||
		existing.UserID != version.UserID ||
		existing.DisplayVersion != version.DisplayVersion {
		return CardVersion{}, ErrVersionConflict
	}
	return existing, nil
}

func (repository *PostgresVersionRepository) Find(
	ctx context.Context,
	userID string,
	cardID string,
	versionID string,
) (CardVersion, error) {
	return scanVersion(repository.database.QueryRowContext(
		ctx,
		versionSelect+` WHERE user_id = $1 AND card_id = $2 AND version_id = $3`,
		userID,
		cardID,
		versionID,
	))
}

func (repository *PostgresVersionRepository) ListByUser(
	ctx context.Context,
	userID string,
) ([]CardVersion, error) {
	return repository.list(ctx, versionSelect+` WHERE user_id = $1 ORDER BY created_at DESC`, userID)
}

func (repository *PostgresVersionRepository) ListByCard(
	ctx context.Context,
	userID string,
	cardID string,
) ([]CardVersion, error) {
	return repository.list(
		ctx,
		versionSelect+` WHERE user_id = $1 AND card_id = $2 ORDER BY created_at DESC`,
		userID,
		cardID,
	)
}

func (repository *PostgresVersionRepository) list(
	ctx context.Context,
	query string,
	arguments ...any,
) ([]CardVersion, error) {
	rows, err := repository.database.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := make([]CardVersion, 0)
	for rows.Next() {
		version, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

type versionScanner interface {
	Scan(...any) error
}

func scanVersion(row versionScanner) (CardVersion, error) {
	var version CardVersion
	var preview []byte
	if err := row.Scan(
		&version.VersionID,
		&version.CardID,
		&version.UserID,
		&version.Runtime,
		&version.DisplayVersion,
		&version.Title,
		&version.Description,
		&version.ArtifactKey,
		&version.ArtifactSHA256,
		&version.KeyID,
		&preview,
		&version.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CardVersion{}, ErrNotFound
		}
		return CardVersion{}, err
	}
	if err := json.Unmarshal(preview, &version.Preview); err != nil {
		return CardVersion{}, fmt.Errorf("decode card preview: %w", err)
	}
	return version, nil
}

const versionSelect = `SELECT version_id, card_id, user_id, runtime,
 display_version, title, description, artifact_object_key, artifact_sha256,
 key_id, preview_json, created_at FROM card_versions`
