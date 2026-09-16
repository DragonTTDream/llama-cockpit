package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const modelsDir = "/home/user/models"
const customUnit = "llama-custom"

type CustomReq struct {
	ModelFile string            `json:"model_file"`
	Port      int               `json:"port"`
	Params    map[string]string `json:"params"`
}

type ModelFile struct {
	File    string `json:"file"`
	SizeMiB int64  `json:"size_mib"`
}

func customUnitDef() UnitDef {
	return UnitDef{
		Name:     "llama-custom",
		Label:    "自定义",
		Model:    "",
		Alias:    "llama-custom",
		Port:     0,
		Embed:    false,
		Keys:     []string{"MODEL", "PORT", "ALIAS", "NGL", "CTX", "EXTRA"},
		Defaults: map[string]string{"NGL": "999", "CTX": "32768", "ALIAS": "llama-custom", "EXTRA": ""},
	}
}

func listModelFiles() ([]ModelFile, error) {
	pattern := modelsDir + "/*.gguf"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var files []ModelFile
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		sizeMiB := info.Size() / 1024 / 1024
		filename := filepath.Base(path)
		files = append(files, ModelFile{File: filename, SizeMiB: sizeMiB})
	}

	// Sort by filename
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			if files[i].File > files[j].File {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

	return files, nil
}

func customLoad(req CustomReq) error {
	u := unitDefByName(customUnit)
	if u == nil {
		return fmt.Errorf("缺少 %s 的配置", customUnit)
	}
	if req.ModelFile == "" || strings.Contains(req.ModelFile, "/") || strings.Contains(req.ModelFile, "..") {
		return fmt.Errorf("模型文件名非法")
	}
	full := modelsDir + "/" + req.ModelFile
	if _, err := os.Stat(full); err != nil {
		return fmt.Errorf("模型文件不存在：%s", req.ModelFile)
	}
	if req.Port < 1024 || req.Port > 65535 {
		return fmt.Errorf("端口必须在 1024-65535")
	}
	for _, other := range unitDefs {
		if other.Name == customUnit {
			continue
		}
		if other.Port == req.Port {
			return fmt.Errorf("端口 %d 已被 %s 占用", req.Port, other.Name)
		}
	}
	values := make(map[string]string, len(u.Keys))
	for _, k := range u.Keys {
		values[k] = ""
	}
	for k, v := range u.Defaults {
		if _, ok := values[k]; ok {
			values[k] = v
		}
	}
	values["MODEL"] = full
	values["PORT"] = strconv.Itoa(req.Port)
	values["ALIAS"] = customUnit
	for k, v := range req.Params {
		if _, ok := values[k]; !ok {
			continue
		}
		values[k] = v
	}
	return applyValues(u, values, true)
}

func customStop() error {
	return unitAction(customUnit, "stop")
}

func customStatus() (bool, int) {
	active := unitActive(customUnit)
	env, err := readEnv(customUnit)
	if err != nil {
		return active, 0
	}
	portStr := env["PORT"]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return active, 0
	}
	return active, port
}

func chatOnce(port int, message string, maxTokens int) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", port)
	body := map[string]interface{}{
		"model":      "local",
		"messages":   []map[string]interface{}{{"role": "user", "content": message}},
		"max_tokens": maxTokens,
		"stream":     false,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("模型返回 %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("模型未返回结果")
	}

	return result.Choices[0].Message.Content, nil
}

func streamChat(ctx context.Context, port int, message string, maxTokens int, onDelta func(string) error) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", port)
	body := map[string]interface{}{
		"model":      "local",
		"messages":   []map[string]interface{}{{"role": "user", "content": message}},
		"max_tokens": maxTokens,
		"stream":     true,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("模型返回 %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		content := strings.TrimPrefix(line, "data: ")
		if content == "[DONE]" {
			break
		}

		var result struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		err = json.Unmarshal([]byte(content), &result)
		if err != nil {
			continue
		}

		if len(result.Choices) > 0 && result.Choices[0].Delta.Content != "" {
			err = onDelta(result.Choices[0].Delta.Content)
			if err != nil {
				return err
			}
		}
	}

	return scanner.Err()
}
