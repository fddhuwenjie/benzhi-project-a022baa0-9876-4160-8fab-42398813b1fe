package store

import "iceguard/internal/domain"

func (s *Store) ByStatus(status domain.BatchStatus) []domain.IceCoreBatch {
	all := s.List()
	out := make([]domain.IceCoreBatch, 0)
	for _, b := range all {
		if b.Status == status {
			out = append(out, b)
		}
	}
	return out
}
