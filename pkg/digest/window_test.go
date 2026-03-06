package digest

import (
	"testing"
	"time"
)

func mustTime(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestCalculateWindow_RelativeDays(t *testing.T) {
	ref := mustTime("2025-03-10")

	t.Run("7d", func(t *testing.T) {
		w, err := CalculateWindow("7d", ref, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := mustTime("2025-03-03")
		if !w.From.Equal(want) {
			t.Errorf("From: got %v, want %v", w.From, want)
		}
		if !w.To.Equal(ref) {
			t.Errorf("To: got %v, want %v", w.To, ref)
		}
	})

	t.Run("30d", func(t *testing.T) {
		w, err := CalculateWindow("30d", ref, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := mustTime("2025-02-08")
		if !w.From.Equal(want) {
			t.Errorf("From: got %v, want %v", w.From, want)
		}
		if !w.To.Equal(ref) {
			t.Errorf("To: got %v, want %v", w.To, ref)
		}
	})

	t.Run("1d", func(t *testing.T) {
		w, err := CalculateWindow("1d", ref, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := mustTime("2025-03-09")
		if !w.From.Equal(want) {
			t.Errorf("From: got %v, want %v", w.From, want)
		}
	})
}

func TestCalculateWindow_RelativeMonths(t *testing.T) {
	ref := mustTime("2025-03-10")

	t.Run("1m", func(t *testing.T) {
		w, err := CalculateWindow("1m", ref, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := mustTime("2025-02-10")
		if !w.From.Equal(want) {
			t.Errorf("From: got %v, want %v", w.From, want)
		}
		if !w.To.Equal(ref) {
			t.Errorf("To: got %v, want %v", w.To, ref)
		}
	})

	t.Run("3m", func(t *testing.T) {
		w, err := CalculateWindow("3m", ref, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := mustTime("2024-12-10")
		if !w.From.Equal(want) {
			t.Errorf("From: got %v, want %v", w.From, want)
		}
	})
}

func TestCalculateWindow_SinceLastRun(t *testing.T) {
	ref := mustTime("2025-03-10")

	t.Run("with lastRunAt", func(t *testing.T) {
		last := mustTime("2025-03-01")
		w, err := CalculateWindow("since_last_run", ref, &last)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !w.From.Equal(last) {
			t.Errorf("From: got %v, want %v", w.From, last)
		}
		if !w.To.Equal(ref) {
			t.Errorf("To: got %v, want %v", w.To, ref)
		}
	})

	t.Run("nil lastRunAt returns error", func(t *testing.T) {
		_, err := CalculateWindow("since_last_run", ref, nil)
		if err == nil {
			t.Fatal("expected error for nil lastRunAt, got nil")
		}
	})
}

func TestCalculateWindow_JSONAbsoluteRange(t *testing.T) {
	ref := mustTime("2025-03-10")

	t.Run("valid range", func(t *testing.T) {
		expr := `{"from":"2025-01-01","to":"2025-01-07"}`
		w, err := CalculateWindow(expr, ref, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantFrom := mustTime("2025-01-01")
		wantToBase := mustTime("2025-01-07")
		wantTo := wantToBase.Add(24*time.Hour - time.Nanosecond)

		if !w.From.Equal(wantFrom) {
			t.Errorf("From: got %v, want %v", w.From, wantFrom)
		}
		if !w.To.Equal(wantTo) {
			t.Errorf("To: got %v, want %v", w.To, wantTo)
		}
	})

	t.Run("to is end-of-day inclusive", func(t *testing.T) {
		expr := `{"from":"2025-03-01","to":"2025-03-05"}`
		w, err := CalculateWindow(expr, ref, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		base := mustTime("2025-03-05")
		wantTo := base.Add(24*time.Hour - time.Nanosecond)
		if !w.To.Equal(wantTo) {
			t.Errorf("To end-of-day: got %v, want %v", w.To, wantTo)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		_, err := CalculateWindow(`{"from":"bad-date","to":"2025-01-07"}`, ref, nil)
		if err == nil {
			t.Fatal("expected error for bad from date, got nil")
		}
	})

	t.Run("bad to date returns error", func(t *testing.T) {
		_, err := CalculateWindow(`{"from":"2025-01-01","to":"not-a-date"}`, ref, nil)
		if err == nil {
			t.Fatal("expected error for bad to date, got nil")
		}
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		_, err := CalculateWindow(`{not valid json`, ref, nil)
		if err == nil {
			t.Fatal("expected error for malformed JSON, got nil")
		}
	})
}

func TestCalculateWindow_ErrorCases(t *testing.T) {
	ref := mustTime("2025-03-10")

	t.Run("unsupported expression", func(t *testing.T) {
		_, err := CalculateWindow("unknown", ref, nil)
		if err == nil {
			t.Fatal("expected error for unsupported expression, got nil")
		}
	})

	t.Run("empty string", func(t *testing.T) {
		_, err := CalculateWindow("", ref, nil)
		if err == nil {
			t.Fatal("expected error for empty expression, got nil")
		}
	})

	t.Run("d without number", func(t *testing.T) {
		_, err := CalculateWindow("d", ref, nil)
		if err == nil {
			t.Fatal("expected error for 'd' without number, got nil")
		}
	})

	t.Run("m without number", func(t *testing.T) {
		_, err := CalculateWindow("m", ref, nil)
		if err == nil {
			t.Fatal("expected error for 'm' without number, got nil")
		}
	})

	t.Run("whitespace trimmed", func(t *testing.T) {
		w, err := CalculateWindow("  7d  ", ref, nil)
		if err != nil {
			t.Fatalf("unexpected error after trim: %v", err)
		}
		want := mustTime("2025-03-03")
		if !w.From.Equal(want) {
			t.Errorf("From: got %v, want %v", w.From, want)
		}
	})
}
