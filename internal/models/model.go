// internal/models/model.go
package models

import (
	"context"
)

// Model is the interface all model backends must implement
type Model interface {
	// Info returns display information about the model
	Info() ModelInfo

	// Send sends a prompt with conversation history and returns a channel of chunks
	Send(ctx context.Context, history []Message, prompt string) <-chan Chunk

	// Stop interrupts any in-progress generation
	Stop()

	// Status returns the current status of the model
	Status() ModelStatus

	// SetStatus updates the model status
	SetStatus(status ModelStatus)
}

// BaseModel provides common functionality for all models
type BaseModel struct {
	info   ModelInfo
	status ModelStatus
}

func NewBaseModel(info ModelInfo) BaseModel {
	return BaseModel{
		info:   info,
		status: StatusIdle,
	}
}

func (m *BaseModel) Info() ModelInfo {
	return m.info
}

func (m *BaseModel) Status() ModelStatus {
	return m.status
}

func (m *BaseModel) SetStatus(status ModelStatus) {
	m.status = status
}
