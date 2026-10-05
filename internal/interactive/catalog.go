package interactive

import (
	"fmt"
	"xmlmerge/internal/rules"
)

// SaveDatabase edits the standalone catalog without requiring sample documents.
// Uniqueness of keys is checked later against actual inputs during preparation.
func (s *Session) SaveDatabase(db *rules.Database, expected string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commit.Lock()
	defer s.commit.Unlock()
	if !s.Options.StandaloneRules() || s.closed.Load() {
		return s.state(), fmt.Errorf("редактор базы правил недоступен")
	}
	if db == nil {
		return s.state(), fmt.Errorf("не передана база правил")
	}
	if err := db.Validate(); err != nil {
		return s.state(), err
	}
	err := rules.Update(s.Options.Rules, func(current *rules.Database) error {
		actual, err := revision(s.Options.Rules)
		if err != nil {
			return err
		}
		if expected != s.revision || actual != expected {
			return fmt.Errorf("настройки изменены другим процессом; обновите их перед сохранением")
		}
		*current = *rules.Clone(db)
		return nil
	})
	if err == nil {
		err = s.refresh(false)
	}
	return s.state(), err
}
