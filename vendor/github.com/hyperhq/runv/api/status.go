package api

import "time"

type Result interface {
	ResultId() string
	IsSuccess() bool
	Message() string
}

type ResultBase struct {
	Id            string
	Success       bool
	ResultMessage string
}

type ProcessExit struct {
	Id         string
	Code       int
	FinishedAt time.Time
}

func NewResultBase(id string, success bool, message string) *ResultBase {
	return &ResultBase{
		Id:            id,
		Success:       success,
		ResultMessage: message,
	}
}

func (r *ResultBase) ResultId() string {
	return r.Id
}

func (r *ResultBase) IsSuccess() bool {
	return r.Success
}

func (r *ResultBase) Message() string {
	return r.ResultMessage
}
