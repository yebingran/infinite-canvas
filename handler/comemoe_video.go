package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"strconv"
	"strings"
)

func normalizeComemoeVideoBody(body []byte, contentType string, modelName string) ([]byte, error) {
	payload := map[string]any{}
	if strings.HasPrefix(strings.ToLower(contentType), "multipart/form-data") {
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return nil, err
		}
		form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(32 << 20)
		if err != nil {
			return nil, err
		}
		defer form.RemoveAll()
		for key, values := range form.Value {
			items := make([]any, 0, len(values))
			for _, value := range values {
				items = append(items, parseKIEFormValue(value))
			}
			if len(items) == 1 {
				payload[key] = items[0]
			} else if len(items) > 1 {
				payload[key] = items
			}
		}
		for key, files := range form.File {
			if key == "video_reference[]" || key == "audio_reference[]" {
				return nil, errors.New("算力萌视频接口当前适配文生和图片参考生视频，不支持视频或音频参考")
			}
			for _, file := range files {
				reader, err := file.Open()
				if err != nil {
					return nil, err
				}
				data, readErr := io.ReadAll(reader)
				_ = reader.Close()
				if readErr != nil {
					return nil, readErr
				}
				contentType := strings.TrimSpace(file.Header.Get("Content-Type"))
				if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
					return nil, errors.New("算力萌参考文件必须是图片")
				}
				encoded := "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
				payload[key] = appendComemoeImage(payload[key], encoded)
			}
		}
	} else if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	for _, key := range []string{"video_reference[]", "video_urls", "videos", "audio_reference[]", "audio_urls", "audios"} {
		if len(comemoeImageValues(payload[key])) > 0 {
			return nil, errors.New("算力萌视频接口当前适配文生和图片参考生视频，不支持视频或音频参考")
		}
	}

	result := map[string]any{
		"model":  strings.TrimSpace(modelName),
		"prompt": strings.TrimSpace(toStringSafe(payload["prompt"])),
	}
	if result["model"] == "" {
		result["model"] = strings.TrimSpace(toStringSafe(payload["model"]))
	}
	if duration := comemoeDuration(payload); duration > 0 {
		result["duration"] = duration
	}
	if ratio := comemoeAspectRatio(payload); ratio != "" {
		result["aspect_ratio"] = ratio
	}
	if resolution := comemoeResolution(payload); resolution != "" {
		result["resolution"] = resolution
	}

	images := []string{}
	for _, key := range []string{"image_url", "image_urls", "input_reference[]", "images", "image", "first_frame_url", "first_frame"} {
		images = append(images, comemoeImageValues(payload[key])...)
	}
	if len(images) == 1 {
		result["image_url"] = images[0]
	} else if len(images) > 1 {
		result["image_urls"] = images
	}
	return json.Marshal(result)
}

func appendComemoeImage(existing any, image string) any {
	if existing == nil {
		return image
	}
	if values, ok := existing.([]any); ok {
		return append(values, image)
	}
	return []any{existing, image}
}

func comemoeDuration(payload map[string]any) int {
	for _, key := range []string{"duration", "seconds"} {
		value := strings.TrimSpace(toStringSafe(payload[key]))
		value = strings.TrimSuffix(strings.ToLower(value), "s")
		if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
			return seconds
		}
	}
	return 0
}

func comemoeAspectRatio(payload map[string]any) string {
	value := strings.TrimSpace(firstNonEmpty(toStringSafe(payload["aspect_ratio"]), toStringSafe(payload["size"])))
	if strings.Contains(value, ":") {
		return value
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == 'x' || r == 'X' || r == '*' })
	if len(parts) != 2 {
		return ""
	}
	width, widthErr := strconv.ParseFloat(parts[0], 64)
	height, heightErr := strconv.ParseFloat(parts[1], 64)
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return ""
	}
	ratio := width / height
	choices := []struct {
		value string
		ratio float64
	}{{"1:1", 1}, {"16:9", 16.0 / 9}, {"9:16", 9.0 / 16}}
	best := choices[0]
	distance := 1e9
	for _, choice := range choices {
		current := ratio - choice.ratio
		if current < 0 {
			current = -current
		}
		if current < distance {
			best, distance = choice, current
		}
	}
	return best.value
}

func comemoeResolution(payload map[string]any) string {
	value := strings.TrimSpace(firstNonEmpty(toStringSafe(payload["resolution"]), toStringSafe(payload["resolution_name"]), toStringSafe(payload["vquality"])))
	switch strings.ToLower(value) {
	case "standard", "480", "480p":
		return "standard"
	case "720", "720p":
		return "720P"
	case "1080", "1080p":
		return "1080P"
	default:
		return value
	}
}

func comemoeImageValues(value any) []string {
	switch typed := value.(type) {
	case string:
		value := strings.TrimSpace(typed)
		if value == "" {
			return nil
		}
		if strings.HasPrefix(value, "[") {
			var decoded []any
			if json.Unmarshal([]byte(value), &decoded) == nil {
				return comemoeImageValues(decoded)
			}
		}
		return []string{value}
	case []any:
		values := []string{}
		for _, item := range typed {
			values = append(values, comemoeImageValues(item)...)
		}
		return values
	case map[string]any:
		return comemoeImageValues(firstNonEmpty(toStringSafe(typed["url"]), toStringSafe(typed["image_url"]), toStringSafe(typed["data_url"])))
	default:
		return nil
	}
}
