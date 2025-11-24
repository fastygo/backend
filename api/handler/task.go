package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"

	"github.com/e-go/backend/api/transport"
	"github.com/e-go/backend/domain"
	"github.com/e-go/backend/pkg/httpcontext"
	"github.com/e-go/backend/repository"
	taskUC "github.com/e-go/backend/usecase/task"
)

type TaskHandler struct {
	baseHandler
	uc *taskUC.UseCase
}

func NewTaskHandler(uc *taskUC.UseCase, adapter *httpcontext.Adapter, logger *zap.Logger) *TaskHandler {
	return &TaskHandler{
		baseHandler: newBaseHandler(adapter, logger),
		uc:          uc,
	}
}

// @Summary List tasks
// @Tags tasks
// @Router /api/v1/tasks [get]
func (h *TaskHandler) GetTasks(ctx *fasthttp.RequestCtx) {
	userID := h.userID(ctx)
	if userID == "" {
		return
	}

	filter := repository.TaskFilter{
		UserID: userID,
		Status: string(ctx.QueryArgs().Peek("status")),
		Limit:  parseInt(string(ctx.QueryArgs().Peek("limit")), 50),
		Offset: parseInt(string(ctx.QueryArgs().Peek("offset")), 0),
	}

	stdCtx, cancel := h.requestContext(ctx)
	defer cancel()

	tasks, err := h.uc.ListTasks(stdCtx, filter)
	if err != nil {
		h.respondError(ctx, err)
		return
	}
	h.respondSuccess(ctx, http.StatusOK, tasks)
}

// @Summary Create task
// @Tags tasks
// @Router /api/v1/tasks [post]
func (h *TaskHandler) CreateTask(ctx *fasthttp.RequestCtx) {
	userID := h.userID(ctx)
	if userID == "" {
		return
	}

	task, ok := h.parseTask(ctx, userID)
	if !ok {
		return
	}

	stdCtx, cancel := h.requestContext(ctx)
	defer cancel()

	created, err := h.uc.CreateTask(stdCtx, task)
	if err != nil {
		h.respondError(ctx, err)
		return
	}
	h.respondSuccess(ctx, http.StatusCreated, created)
}

// @Summary Update task
// @Tags tasks
// @Router /api/v1/tasks/{id} [put]
func (h *TaskHandler) UpdateTask(ctx *fasthttp.RequestCtx) {
	userID := h.userID(ctx)
	if userID == "" {
		return
	}

	task, ok := h.parseTask(ctx, userID)
	if !ok {
		return
	}

	if task.ID == "" {
		if id, ok := ctx.UserValue("id").(string); ok {
			task.ID = id
		}
	}

	stdCtx, cancel := h.requestContext(ctx)
	defer cancel()

	updated, err := h.uc.UpdateTask(stdCtx, task)
	if err != nil {
		h.respondError(ctx, err)
		return
	}
	h.respondSuccess(ctx, http.StatusOK, updated)
}

// @Summary Delete task
// @Tags tasks
// @Router /api/v1/tasks/{id} [delete]
func (h *TaskHandler) DeleteTask(ctx *fasthttp.RequestCtx) {
	userID := h.userID(ctx)
	if userID == "" {
		return
	}

	id, _ := ctx.UserValue("id").(string)
	if id == "" {
		h.respondJSON(ctx, http.StatusBadRequest, transport.NewError(string(domain.ErrCodeInvalid), "missing task id", nil))
		return
	}

	stdCtx, cancel := h.requestContext(ctx)
	defer cancel()

	if err := h.uc.DeleteTask(stdCtx, id); err != nil {
		h.respondError(ctx, err)
		return
	}
	h.respondSuccess(ctx, http.StatusNoContent, nil)
}

func (h *TaskHandler) parseTask(ctx *fasthttp.RequestCtx, userID string) (*domain.Task, bool) {
	var req transport.TaskRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		h.respondJSON(ctx, http.StatusBadRequest, transport.NewError(string(domain.ErrCodeInvalid), "invalid payload", nil))
		return nil, false
	}

	var due *time.Time
	if req.DueDate != "" {
		if parsed, err := time.Parse(time.RFC3339, req.DueDate); err == nil {
			due = &parsed
		}
	}

	task := &domain.Task{
		ID:          req.ID,
		UserID:      userID,
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		Priority:    req.Priority,
		DueDate:     due,
		Metadata:    req.Metadata,
	}

	if task.Status == "" {
		task.Status = "pending"
	}

	return task, true
}

func (h *TaskHandler) userID(ctx *fasthttp.RequestCtx) string {
	userID := string(ctx.Request.Header.Peek("X-User-ID"))
	if userID == "" {
		h.respondJSON(ctx, http.StatusUnauthorized, transport.NewError(string(domain.ErrCodeUnauthorized), "missing user id", nil))
	}
	return userID
}

func parseInt(value string, fallback int) int {
	if v, err := strconv.Atoi(value); err == nil {
		return v
	}
	return fallback
}

