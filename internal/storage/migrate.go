package storage

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate выполняет миграции базы данных.
// Использует PRAGMA user_version для отслеживания применённых миграций.
func Migrate(db *sql.DB, logger *slog.Logger) error {
	// получаем текущую версию схемы
	var currentVersion int
	err := db.QueryRow("PRAGMA user_version").Scan(&currentVersion)
	if err != nil {
		return errors.Join(errors.New("не удалось получить user_version"), err)
	}

	logger.Info("текущая версия схемы", "version", currentVersion)

	// список миграций (индекс = версия - 1)
	migrations := []string{
		"migrations/001_init.sql",
	}

	// применяем миграции по порядку
	for i := currentVersion; i < len(migrations); i++ {
		migrationFile := migrations[i]
		targetVersion := i + 1

		logger.Info("применяю миграцию", "file", migrationFile, "target_version", targetVersion)

		content, err := migrationsFS.ReadFile(migrationFile)
		if err != nil {
			return errors.Join(errors.New("не удалось прочитать миграцию "+migrationFile), err)
		}

		// выполняем миграцию в транзакции
		tx, err := db.Begin()
		if err != nil {
			return errors.Join(errors.New("не удалось начать транзакцию"), err)
		}

		_, err = tx.Exec(string(content))
		if err != nil {
			_ = tx.Rollback() //nolint:errcheck,gosec // rollback после ошибки
			return errors.Join(errors.New("не удалось выполнить миграцию "+migrationFile), err)
		}

		// обновляем версию (PRAGMA не поддерживает параметризацию)
		_, err = tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", targetVersion))
		if err != nil {
			_ = tx.Rollback() //nolint:errcheck,gosec // rollback после ошибки
			return errors.Join(errors.New("не удалось обновить user_version"), err)
		}

		if err := tx.Commit(); err != nil {
			return errors.Join(errors.New("не удалось закоммитить транзакцию"), err)
		}

		logger.Info("миграция применена", "version", targetVersion)
	}

	return nil
}
