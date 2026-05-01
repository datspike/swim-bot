package storage

import (
	"errors"
	"log/slog"
)

// OpenRuntime открывает SQLite storage и подготавливает его к runtime-работе бота.
func OpenRuntime(dsn string, logger *slog.Logger) (*Storage, error) {
	store, err := NewStorage(dsn, logger)
	if err != nil {
		return nil, err
	}

	if err := Migrate(store.DB(), logger); err != nil {
		return nil, closeAfterOpenError(store, errors.Join(errors.New("не удалось выполнить миграции"), err))
	}
	if err := store.RunStartupMutations(); err != nil {
		return nil, closeAfterOpenError(store, errors.Join(errors.New("не удалось выполнить startup-обновления"), err))
	}

	return store, nil
}

// RunStartupMutations применяет идемпотентные runtime-обновления состояния.
func (s *Storage) RunStartupMutations() error {
	return s.ActivateConfiguredChats()
}

func closeAfterOpenError(store *Storage, err error) error {
	if closeErr := store.Close(); closeErr != nil {
		return errors.Join(err, errors.Join(errors.New("не удалось закрыть базу после ошибки запуска"), closeErr))
	}
	return err
}
