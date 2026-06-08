package usage

import "testing"

func TestIsParsingError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{&notEnoughArgumentsError{}, true},
		{&tooManyArgumentsError{}, true},
		{&argumentMismatchError{}, true},
		{ErrExplainOnly, false},
		{nil, false},
	}
	for _, tt := range tests {
		got := isParsingError(tt.err)
		if got != tt.want {
			t.Errorf("isParsingError(%T) = %v, want %v", tt.err, got, tt.want)
		}
	}
}
