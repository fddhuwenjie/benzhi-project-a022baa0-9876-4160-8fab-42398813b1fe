package httpapi

import "net/http"

func decodeRevision(r *http.Request) int { return expected(r) }
