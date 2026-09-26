package store

import "github.com/imhttran/agentic-sop/internal/domain"

type TaskRepository interface {
	Save(task *domain.Task) error
	Get(id string) (*domain.Task, error)
	List() ([]*domain.Task, error)
}

var ErrNotFound = &notFoundError{}

type notFoundError struct{}

func (e *notFoundError) Error() string {
	return "task not found"
}

func IsNotFound(err error) bool {
	_, ok := err.(*notFoundError)
	return ok
}
