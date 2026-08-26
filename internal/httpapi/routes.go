package httpapi

import "net/http"

func routeCount() int { return 8 }

var _ = http.MethodGet
