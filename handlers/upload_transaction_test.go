// SPDX-License-Identifier: MIT

package handlers

import (
	"errors"
	"reflect"
	"testing"
)

func withTransactionPublisher(t *testing.T, failAt int, events *[]string) {
	t.Helper()
	old := publishUploadFile
	calls := 0
	publishUploadFile = func(_, dst string, _ int64) (publishedFile, error) {
		calls++
		if calls == failAt {
			return publishedFile{}, errors.New("injected publish failure")
		}
		return publishedFile{
			rollbackAction: func() { *events = append(*events, "rollback "+dst) },
			finishAction:   func() { *events = append(*events, "finish "+dst) },
		}, nil
	}
	t.Cleanup(func() { publishUploadFile = old })
}

func TestUploadTransactionFaultMatrix(t *testing.T) {
	t.Run("first publish", func(t *testing.T) {
		var events []string
		withTransactionPublisher(t, 1, &events)
		tx := &uploadTransaction{}
		_ = tx.staged()
		if _, err := tx.publish("s", "media", 1); err == nil {
			t.Fatal("expected failure")
		}
		tx.rollback()
		if len(events) != 0 || tx.phase != uploadRolledBack {
			t.Fatalf("events=%v phase=%v", events, tx.phase)
		}
	})
	t.Run("second publish rolls back first", func(t *testing.T) {
		var events []string
		withTransactionPublisher(t, 2, &events)
		tx := &uploadTransaction{}
		_ = tx.staged()
		_, _ = tx.publish("s1", "media", 1)
		if _, err := tx.publish("s2", "preview", 1); err == nil {
			t.Fatal("expected failure")
		}
		tx.rollback()
		if !reflect.DeepEqual(events, []string{"rollback media"}) {
			t.Fatalf("events=%v", events)
		}
	})
	t.Run("metadata commit failure reverses publish order", func(t *testing.T) {
		var events []string
		withTransactionPublisher(t, 0, &events)
		tx := &uploadTransaction{}
		_ = tx.staged()
		_, _ = tx.publish("s1", "media", 1)
		_, _ = tx.publish("s2", "preview", 1)
		// Simulate storage.Update returning an error by not calling commit.
		tx.rollback()
		if !reflect.DeepEqual(events, []string{"rollback preview", "rollback media"}) {
			t.Fatalf("events=%v", events)
		}
	})
	t.Run("commit finalizes and cannot roll back", func(t *testing.T) {
		var events []string
		withTransactionPublisher(t, 0, &events)
		tx := &uploadTransaction{}
		_ = tx.staged()
		_, _ = tx.publish("s1", "media", 1)
		if err := tx.commit(); err != nil {
			t.Fatal(err)
		}
		if err := tx.finalize(); err != nil {
			t.Fatal(err)
		}
		tx.rollback()
		if !reflect.DeepEqual(events, []string{"finish media"}) {
			t.Fatalf("events=%v", events)
		}
	})
}

func TestUploadTransactionRejectsOutOfOrderBoundaries(t *testing.T) {
	tx := &uploadTransaction{}
	if _, err := tx.publish("s", "d", 1); err == nil {
		t.Fatal("publish before stage accepted")
	}
	if err := tx.commit(); err == nil {
		t.Fatal("commit before publish accepted")
	}
	if err := tx.finalize(); err == nil {
		t.Fatal("finalize before commit accepted")
	}
}
