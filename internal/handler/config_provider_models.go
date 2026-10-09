package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"pyntra/internal/config"

	"github.com/gin-gonic/gin"
)

// ProviderModelsRequest asks the server to list the models a provider endpoint
// actually serves. base_url/api_key are optional; when empty or masked the
// currently stored OpenAI config is used. This proxy runs server-side so the
// browser never needs cross-origin access to local servers (Ollama, LM Studio)
// and never has to hold the API key.
type ProviderModelsRequest struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
}

// GetProviderModels returns the model ids available at the given (or configured)
// endpoint. It understands the OpenAI `/v1/models` shape and the Ollama
// `/api/tags` shape, so local and hosted providers both work.
func (h *ConfigHandler) GetProviderModels(c *gin.Context) {
	var req ProviderModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}

	h.mu.RLock()
	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(h.config.OpenAI.BaseURL)
	}
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" || apiKey == apiKeyMask {
		apiKey = h.config.OpenAI.APIKey
	}
	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = h.config.OpenAI.Provider
	}
	h.mu.RUnlock()

	if baseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "base_url is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	models, err := listProviderModels(ctx, provider, baseURL, apiKey)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error(), "models": []string{}})
		return
	}
	sort.Strings(models)
	c.JSON(http.StatusOK, gin.H{"ok": true, "models": models})
}

func listProviderModels(ctx context.Context, provider, baseURL, apiKey string) ([]string, error) {
	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")

	// Ollama exposes an idiomatic /api/tags endpoint (base is usually .../v1).
	if config.NormalizeProvider(provider) == "ollama" || strings.Contains(base, "11434") {
		ollamaBase := strings.TrimSuffix(base, "/v1")
		if names, err := fetchOllamaTags(ctx, ollamaBase); err == nil && len(names) > 0 {
			return names, nil
		}
		// fall through to the OpenAI-compatible path if tags failed
	}

	return fetchOpenAIModels(ctx, base, apiKey)
}

func fetchOpenAIModels(ctx context.Context, base, apiKey string) ([]string, error) {
	url := base + "/models"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("endpoint returned status %d", resp.StatusCode)
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("unexpected response from %s", url)
	}
	out := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if id := strings.TrimSpace(m.ID); id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

func fetchOllamaTags(ctx context.Context, base string) ([]string, error) {
	url := base + "/api/tags"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}
	var parsed struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		if n := strings.TrimSpace(m.Name); n != "" {
			out = append(out, n)
		}
	}
	return out, nil
}
