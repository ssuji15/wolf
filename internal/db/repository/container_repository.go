package repository

import (
	"context"
	"time"

	"github.com/ssuji15/wolf/internal/db"
	"github.com/ssuji15/wolf/model"
)

type ContainerRepository struct {
	db *db.DB
}

func NewContainerRepository(db *db.DB) *ContainerRepository {
	return &ContainerRepository{db: db}
}

func (c *ContainerRepository) Insert(ctx context.Context, m model.WorkerMetadata) error {
	_, err := c.db.Pool.Exec(ctx,
		`INSERT INTO containers (
            id, name, socket_path, created_at, updated_at, status
        ) VALUES ($1,$2,$3,$4,$5,$6)`,
		m.ID, m.Name, m.SocketPath, m.CreatedAt, m.UpdatedAt, m.Status,
	)
	return err
}

func (c *ContainerRepository) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := c.db.Pool.Exec(ctx,
		`UPDATE containers SET status=$1, updated_at=$2 WHERE id=$3`,
		status, time.Now().UTC(), id,
	)
	return err
}
