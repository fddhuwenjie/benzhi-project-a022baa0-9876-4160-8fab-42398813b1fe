package workflow

import "iceguard/internal/domain"

type CommandResult struct {
	Batch   domain.IceCoreBatch `json:"batch"`
	Message string              `json:"message"`
}

func result(b domain.IceCoreBatch, msg string) CommandResult {
	return CommandResult{Batch: b, Message: msg}
}
