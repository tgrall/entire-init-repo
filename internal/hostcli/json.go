package hostcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// DecodeFirstJSON decodes the first JSON value in data and ignores anything
// after it. Older `entire` releases print an "Update available!" banner on
// stdout after --json output; that trailing chatter must not fail parsing.
// Leading garbage is still an error.
func DecodeFirstJSON(data []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("empty output, expected JSON")
		}
		return err
	}
	return nil
}
