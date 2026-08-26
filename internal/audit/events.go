package audit

import "time"

func NewEvent(batchID, typ string, revision int, payload any, previous string) Event {
	return Event{ID: time.Now().UTC().Format("20060102150405.000000000"), BatchID: batchID, Type: typ, Revision: revision, Payload: payload, PreviousDigest: previous, At: time.Now().UTC()}
}
