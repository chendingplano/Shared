package loggerutil

import "testing"

func TestAlertSinkReceivesWarnAndError(t *testing.T) {
	var got []AlertRecord
	SetAlertSink(func(r AlertRecord) { got = append(got, r) })
	defer SetAlertSink(nil)

	l := CreateDefaultLogger("TEST_LOC")
	l.Info("ignored")
	l.Warn("w", "k", 1)
	l.Error("e")

	if len(got) != 2 || got[0].Level != "WARN" || got[1].Level != "ERROR" {
		t.Fatalf("unexpected records: %+v", got)
	}
	if got[0].Attrs["k"] != "1" || got[0].Loc != "TEST_LOC" || got[0].Message != "w" {
		t.Fatalf("bad record: %+v", got[0])
	}
}
