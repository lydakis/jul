package metadata

import "github.com/oklog/ulid/v2"

func newID() string {
	return ulid.Make().String()
}
