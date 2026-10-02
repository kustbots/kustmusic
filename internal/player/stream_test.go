package player

import (
	"errors"
	"testing"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		err          error
		expectedKind errorKind
	}{
		{errors.New("group call is closed"), errFatal},
		{errors.New("GROUPCALL_FORBIDDEN"), errFatal},
		{errors.New("GROUPCALL_INVALID"), errFatal},
		{errors.New("GROUPCALL_ADD_PARTICIPANTS_FAILED"), errRetryOnce},
		{errors.New("CHANNELS_TOO_MUCH"), errRotate},
		{errors.New("FROZEN_METHOD_INVALID"), errRotate},
		{errors.New("FLOOD_WAIT_X"), errRotate},
		{errors.New("USER_DEACTIVATED"), errRotate},
		{errors.New("some unknown error"), errUnknown},
	}

	for _, tt := range tests {
		kind := classifyError(tt.err)
		if kind != tt.expectedKind {
			t.Errorf("classifyError(%v) = %v, want %v", tt.err, kind, tt.expectedKind)
		}
	}
}

func TestFatalMessage(t *testing.T) {
	errClosed := errors.New("group call is closed")
	msgClosed := fatalMessage(errClosed)
	if msgClosed.Error() != "<b>No active video chat found.</b>\n\nPlease start one and <b>try again</b>" {
		t.Errorf("unexpected fatalMessage for closed: %v", msgClosed)
	}

	errInvalid := errors.New("GROUPCALL_INVALID")
	msgInvalid := fatalMessage(errInvalid)
	if msgInvalid.Error() != "<b>GROUPCALL_INVALID:</b> start a video chat and try again.\n\nIf the problem persists, please report it to the developer." {
		t.Errorf("unexpected fatalMessage for invalid: %v", msgInvalid)
	}

	errOther := errors.New("other error")
	msgOther := fatalMessage(errOther)
	if !errors.Is(msgOther, errOther) {
		t.Errorf("expected original error returned, got %v", msgOther)
	}
}
