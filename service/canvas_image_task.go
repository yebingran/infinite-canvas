package service

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/repository"
)

type CanvasImageTaskCreateInput struct {
	UserID          string
	UserDisplayName string
	Source          string
	SourceID        string
	NodeID          string
	ClientTaskID    string
	Model           string
	ChannelID       string
	UserChannelID   string
	ChannelName     string
	WorkflowRef     string
	Credits         float64
	BillingName     string
	BillingPath     string
	Prompt          string
	GenerationType  string
	Endpoint        string
	ContentType     string
	RequestBody     string
}

func CreateCanvasImageTask(input CanvasImageTaskCreateInput) (model.CanvasImageTask, error) {
	current := now()
	task := model.CanvasImageTask{
		ID:              firstVideoTaskValue(input.ClientTaskID, "canvas_image_task_"+uuid.NewString()),
		UserID:          strings.TrimSpace(input.UserID),
		UserDisplayName: strings.TrimSpace(input.UserDisplayName),
		Source:          normalizeCanvasImageTaskSource(input.Source),
		SourceID:        strings.TrimSpace(input.SourceID),
		NodeID:          strings.TrimSpace(input.NodeID),
		Model:           strings.TrimSpace(input.Model),
		ChannelID:       strings.TrimSpace(input.ChannelID),
		UserChannelID:   strings.TrimSpace(input.UserChannelID),
		ChannelName:     strings.TrimSpace(input.ChannelName),
		WorkflowRef:     input.WorkflowRef,
		Credits:         input.Credits,
		Status:          "queued",
		Progress:        0,
		Prompt:          strings.TrimSpace(input.Prompt),
		GenerationType:  strings.TrimSpace(input.GenerationType),
		Endpoint:        strings.TrimSpace(input.Endpoint),
		ContentType:     strings.TrimSpace(input.ContentType),
		RequestBody:     input.RequestBody,
		CreatedAt:       current,
		UpdatedAt:       current,
	}
	saved := task
	var err error
	if input.WorkflowRef != "" {
		err = ConsumeUserCredits(task.UserID, input.BillingName, task.Credits, input.BillingPath, &task)
	} else {
		saved, err = repository.SaveCanvasImageTask(task)
	}
	if err == nil {
		WakeVideoTaskPoller()
	}
	return saved, err
}

func GetUserCanvasImageTask(userID string, id string) (model.CanvasImageTask, bool, error) {
	task, found, err := repository.GetUserCanvasImageTask(strings.TrimSpace(userID), strings.TrimSpace(id))
	if err != nil || !found {
		return task, found, err
	}
	task, err = reconcileCanvasImageTask(task)
	return task, true, err
}

func ListUserCanvasImageTasks(userID string, sources []string, limit int) ([]map[string]any, error) {
	tasks, err := repository.ListUserCanvasImageTasks(strings.TrimSpace(userID), normalizeCanvasImageTaskSources(sources), limit)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		task, err = reconcileCanvasImageTask(task)
		if err != nil {
			return nil, err
		}
		result = append(result, CanvasImageTaskResponse(task))
	}
	return result, nil
}

func BatchUserCanvasImageTasks(userID string, ids []string) ([]map[string]any, error) {
	tasks, err := repository.BatchUserCanvasImageTasks(strings.TrimSpace(userID), ids)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		task, err = reconcileCanvasImageTask(task)
		if err != nil {
			return nil, err
		}
		result = append(result, CanvasImageTaskResponse(task))
	}
	return result, nil
}

const canvasImageTaskStaleAfter = 2 * time.Minute

func reconcileCanvasImageTask(task model.CanvasImageTask) (model.CanvasImageTask, error) {
	switch strings.ToLower(strings.TrimSpace(task.Status)) {
	case "queued", "processing", "running", "in_progress":
	default:
		return task, nil
	}
	if CanvasTaskIsRunning("image", task.UserID, task.ID) {
		return task, nil
	}
	lastActivity := strings.TrimSpace(task.UpdatedAt)
	if lastActivity == "" {
		lastActivity = strings.TrimSpace(task.StartedAt)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, lastActivity)
	if err != nil || time.Since(updatedAt) < canvasImageTaskStaleAfter {
		return task, nil
	}
	completedAt := now()
	message := "图片任务已中断，请重新生成。"
	detail := "服务端已不再运行此任务，可能由服务重启或任务进程中断导致。"
	updated, err := repository.FailOrphanedCanvasImageTask(task.UserID, task.ID, completedAt, message, detail)
	if err != nil {
		return task, err
	}
	if updated {
		task.Status, task.CompletedAt, task.UpdatedAt, task.Error, task.ErrorDetail = "failed", completedAt, completedAt, message, detail
		return task, nil
	}
	latest, found, err := repository.GetUserCanvasImageTask(task.UserID, task.ID)
	if err != nil || !found {
		return task, err
	}
	return latest, nil
}

func DeleteUserCanvasImageTask(userID string, id string) error {
	return repository.DeleteUserCanvasImageTask(strings.TrimSpace(userID), strings.TrimSpace(id))
}

func SaveCanvasImageTask(task model.CanvasImageTask) (model.CanvasImageTask, error) {
	task.UpdatedAt = now()
	return repository.UpdateCanvasImageTask(task)
}

func CanvasImageTaskResponse(task model.CanvasImageTask) map[string]any {
	result := map[string]any{
		"id":             task.ID,
		"object":         "canvas.image.task",
		"source":         task.Source,
		"source_id":      task.SourceID,
		"node_id":        task.NodeID,
		"model":          task.Model,
		"status":         task.Status,
		"progress":       task.Progress,
		"prompt":         task.Prompt,
		"generationType": task.GenerationType,
		"created_at":     task.CreatedAt,
		"updated_at":     task.UpdatedAt,
		"started_at":     task.StartedAt,
		"completed_at":   task.CompletedAt,
		"createdAt":      task.CreatedAt,
		"updatedAt":      task.UpdatedAt,
	}
	if task.ImageURL != "" {
		result["url"] = task.ImageURL
		result["image_url"] = task.ImageURL
		if len(task.ImageURLs) > 0 {
			result["image_urls"] = task.ImageURLs
		}
		result["storageKey"] = task.StorageKey
		result["width"] = task.Width
		result["height"] = task.Height
		result["mimeType"] = task.MimeType
		result["bytes"] = task.Bytes
	}
	if task.WorkflowRef != "" {
		result["workflowRef"] = task.WorkflowRef
	}
	if task.Error != "" || task.ErrorDetail != "" {
		result["error"] = map[string]any{"message": firstVideoTaskValue(task.Error, task.ErrorDetail)}
		result["error_detail"] = task.ErrorDetail
	}
	return result
}

func normalizeCanvasImageTaskSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "image-workbench":
		return "image-workbench"
	case "workflow":
		return "workflow"
	case "canvas", "":
		return "canvas"
	default:
		return "canvas"
	}
}

func normalizeCanvasImageTaskSources(sources []string) []string {
	result := make([]string, 0, len(sources))
	seen := map[string]bool{}
	for _, source := range sources {
		normalized := normalizeCanvasImageTaskSource(source)
		if normalized != "" && !seen[normalized] {
			result = append(result, normalized)
			seen[normalized] = true
		}
	}
	return result
}
