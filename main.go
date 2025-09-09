package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// RegistryClient структура для работы с Docker Registry
type RegistryClient struct {
	BaseURL  string
	Username string
	Password string
	Client   *http.Client
}

// RepositoriesResponse структура ответа со списком репозиториев
type RepositoriesResponse struct {
	Repositories []string `json:"repositories"`
}

// TagsResponse структура ответа со списком тегов
type TagsResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// ManifestResponse структура ответа с манифестом
type ManifestResponse struct {
	SchemaVersion int `json:"schemaVersion"`
	History       []struct {
		V1Compatibility string `json:"v1Compatibility"`
	} `json:"history"`
}

// ManifestV2Response структура ответа с манифестом v2
type ManifestV2Response struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        struct {
		Digest string `json:"digest"`
	} `json:"config"`
}

// ConfigResponse структура ответа с конфигурацией образа
type ConfigResponse struct {
	Created time.Time `json:"created"`
	Config  struct {
		Labels map[string]string `json:"Labels"`
	} `json:"config"`
}

// V1Compatibility структура для парсинга v1 совместимости
type V1Compatibility struct {
	Created time.Time `json:"created"`
}

// ImageInfo информация об образе
type ImageInfo struct {
	Repository string
	Tag        string
	Digest     string
	Created    time.Time
}

// NewRegistryClient создает новый клиент для работы с Registry
func NewRegistryClient(baseURL, username, password string) *RegistryClient {
	return &RegistryClient{
		BaseURL:  strings.TrimSuffix(baseURL, "/"),
		Username: username,
		Password: password,
		Client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// makeRequest выполняет HTTP запрос с аутентификацией
func (rc *RegistryClient) makeRequest(method, url string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	if rc.Username != "" && rc.Password != "" {
		req.SetBasicAuth(rc.Username, rc.Password)
	}

	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	return rc.Client.Do(req)
}

// GetRepositories получает список всех репозиториев
func (rc *RegistryClient) GetRepositories() ([]string, error) {
	url := fmt.Sprintf("%s/v2/_catalog", rc.BaseURL)
	resp, err := rc.makeRequest("GET", url)
	if err != nil {
		return nil, fmt.Errorf("ошибка при получении списка репозиториев: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("получен статус %d при запросе репозиториев", resp.StatusCode)
	}

	var repoResp RepositoriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&repoResp); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа: %v", err)
	}

	return repoResp.Repositories, nil
}

// GetTags получает список тегов для репозитория
func (rc *RegistryClient) GetTags(repository string) ([]string, error) {
	url := fmt.Sprintf("%s/v2/%s/tags/list", rc.BaseURL, repository)
	resp, err := rc.makeRequest("GET", url)
	if err != nil {
		return nil, fmt.Errorf("ошибка при получении тегов для %s: %v", repository, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("получен статус %d при запросе тегов для %s", resp.StatusCode, repository)
	}

	var tagsResp TagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, fmt.Errorf("ошибка декодирования тегов: %v", err)
	}

	return tagsResp.Tags, nil
}

// GetManifestDigest получает digest манифеста
func (rc *RegistryClient) GetManifestDigest(repository, tag string) (string, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", rc.BaseURL, repository, tag)
	resp, err := rc.makeRequest("HEAD", url)
	if err != nil {
		return "", fmt.Errorf("ошибка при получении манифеста для %s:%s: %v", repository, tag, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("получен статус %d при запросе манифеста для %s:%s", resp.StatusCode, repository, tag)
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("digest не найден для %s:%s", repository, tag)
	}

	return digest, nil
}

// GetImageCreated получает время создания образа из манифеста
func (rc *RegistryClient) GetImageCreated(repository, tag string) (time.Time, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", rc.BaseURL, repository, tag)

	// Сначала пробуем получить манифест v1
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return time.Time{}, err
	}

	if rc.Username != "" && rc.Password != "" {
		req.SetBasicAuth(rc.Username, rc.Password)
	}

	// Пробуем получить v1 манифест
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v1+json")
	resp, err := rc.Client.Do(req)
	if err != nil {
		return time.Time{}, fmt.Errorf("ошибка при получении манифеста для %s:%s: %v", repository, tag, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var manifest ManifestResponse
		if err := json.NewDecoder(resp.Body).Decode(&manifest); err == nil && len(manifest.History) > 0 {
			var v1Compat V1Compatibility
			if err := json.Unmarshal([]byte(manifest.History[0].V1Compatibility), &v1Compat); err == nil {
				return v1Compat.Created, nil
			}
		}
	}

	// Если v1 не сработал, пробуем v2
	req, err = http.NewRequest("GET", url, nil)
	if err != nil {
		return time.Time{}, err
	}

	if rc.Username != "" && rc.Password != "" {
		req.SetBasicAuth(rc.Username, rc.Password)
	}

	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	resp, err = rc.Client.Do(req)
	if err != nil {
		return time.Time{}, fmt.Errorf("ошибка при получении v2 манифеста для %s:%s: %v", repository, tag, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var manifestV2 ManifestV2Response
		if err := json.NewDecoder(resp.Body).Decode(&manifestV2); err == nil && manifestV2.Config.Digest != "" {
			// Получаем конфигурацию образа
			configURL := fmt.Sprintf("%s/v2/%s/blobs/%s", rc.BaseURL, repository, manifestV2.Config.Digest)
			configResp, err := rc.makeRequest("GET", configURL)
			if err == nil {
				defer configResp.Body.Close()
				if configResp.StatusCode == http.StatusOK {
					var config ConfigResponse
					if err := json.NewDecoder(configResp.Body).Decode(&config); err == nil {
						return config.Created, nil
					}
				}
			}
		}
	}

	// Если ничего не получилось, возвращаем текущее время как fallback
	fmt.Printf("  Предупреждение: не удалось получить время создания для %s:%s, используем текущее время\n", repository, tag)
	return time.Now(), nil
}

// DeleteManifest удаляет манифест по digest
func (rc *RegistryClient) DeleteManifest(repository, digest string) error {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", rc.BaseURL, repository, digest)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("ошибка создания DELETE запроса: %v", err)
	}

	if rc.Username != "" && rc.Password != "" {
		req.SetBasicAuth(rc.Username, rc.Password)
	}

	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	resp, err := rc.Client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка при удалении манифеста %s: %v", digest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusOK {
		return nil
	}

	// Читаем тело ответа для получения детальной информации об ошибке
	body, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusMethodNotAllowed: // 405
		fmt.Printf("\n🚨 ОШИБКА КОНФИГУРАЦИИ REGISTRY:\n")
		fmt.Printf("Docker Registry не настроен для поддержки удаления образов.\n\n")
		fmt.Printf("📋 Для исправления:\n")
		fmt.Printf("1. Остановите Registry\n")
		fmt.Printf("2. Добавьте в config.yml:\n")
		fmt.Printf("   storage:\n")
		fmt.Printf("     delete:\n")
		fmt.Printf("       enabled: true\n")
		fmt.Printf("3. Перезапустите Registry\n\n")
		fmt.Printf("📄 Подробные инструкции: см. файл REGISTRY_SETUP.md\n\n")
		return fmt.Errorf("удаление не поддерживается Registry (статус 405)")
	case http.StatusNotFound: // 404
		return fmt.Errorf("манифест не найден (статус 404): %s", string(body))
	case http.StatusUnauthorized: // 401
		return fmt.Errorf("ошибка авторизации (статус 401): %s", string(body))
	case http.StatusForbidden: // 403
		return fmt.Errorf("доступ запрещен (статус 403): %s", string(body))
	default:
		return fmt.Errorf("получен статус %d при удалении манифеста: %s", resp.StatusCode, string(body))
	}
}

// CleanupRepository очищает репозиторий, оставляя только 2 самых новых образа
func (rc *RegistryClient) CleanupRepository(repository string, keepLast int) error {
	fmt.Printf("Обработка репозитория: %s\n", repository)

	tags, err := rc.GetTags(repository)
	if err != nil {
		return err
	}

	if len(tags) <= keepLast {
		fmt.Printf("  В репозитории %s только %d тегов, пропускаем\n", repository, len(tags))
		return nil
	}

	var images []ImageInfo

	// Получаем информацию о каждом образе
	for _, tag := range tags {
		digest, err := rc.GetManifestDigest(repository, tag)
		if err != nil {
			fmt.Printf("  Предупреждение: не удалось получить digest для %s:%s: %v\n", repository, tag, err)
			continue
		}

		created, err := rc.GetImageCreated(repository, tag)
		if err != nil {
			fmt.Printf("  Предупреждение: не удалось получить время создания для %s:%s: %v\n", repository, tag, err)
			created = time.Now() // Используем текущее время в качестве запасного варианта
		}

		images = append(images, ImageInfo{
			Repository: repository,
			Tag:        tag,
			Digest:     digest,
			Created:    created,
		})

		fmt.Printf("  Образ %s:%s создан %s\n", repository, tag, created.Format("2006-01-02 15:04:05"))
	}

	// Сортируем по времени создания (новые образы первыми)
	sort.Slice(images, func(i, j int) bool {
		return images[i].Created.After(images[j].Created)
	})

	fmt.Printf("  Образы отсортированы по времени создания (новые первыми):\n")
	for i, img := range images {
		status := "сохранить"
		if i >= keepLast {
			status = "удалить"
		}
		fmt.Printf("    %d. %s:%s (%s) - %s\n", i+1, img.Repository, img.Tag,
			img.Created.Format("2006-01-02 15:04:05"), status)
	}

	// Удаляем все кроме последних keepLast образов
	if len(images) > keepLast {
		toDelete := images[keepLast:]
		fmt.Printf("  Найдено %d образов, сохраняем %d новейших, удаляем %d старых\n",
			len(images), keepLast, len(toDelete))

		for _, img := range toDelete {
			fmt.Printf("  Удаляем %s:%s (создан: %s, digest: %s)\n",
				img.Repository, img.Tag, img.Created.Format("2006-01-02 15:04:05"), img.Digest[:12])
			if err := rc.DeleteManifest(img.Repository, img.Digest); err != nil {
				fmt.Printf("  Ошибка при удалении %s:%s: %v\n", img.Repository, img.Tag, err)
			} else {
				fmt.Printf("  Успешно удален %s:%s\n", img.Repository, img.Tag)
			}
		}
	}

	return nil
}

func main() {
	// Получаем параметры из переменных окружения или используем значения по умолчанию
	registryURL := os.Getenv("REGISTRY_URL")
	if registryURL == "" {
		registryURL = "http://localhost:5000" // Значение по умолчанию
	}

	username := os.Getenv("REGISTRY_USERNAME")
	password := os.Getenv("REGISTRY_PASSWORD")

	keepLast := 2 // Количество образов для сохранения

	fmt.Printf("🐳 Docker Registry Cleaner\n")
	fmt.Printf("Подключение к Docker Registry: %s\n", registryURL)

	client := NewRegistryClient(registryURL, username, password)

	// Получаем список всех репозиториев
	repositories, err := client.GetRepositories()
	if err != nil {
		log.Fatalf("Ошибка при получении списка репозиториев: %v", err)
	}

	if len(repositories) == 0 {
		fmt.Println("Репозитории не найдены")
		return
	}

	fmt.Printf("Найдено %d репозиториев\n", len(repositories))

	// Очищаем каждый репозиторий
	for _, repo := range repositories {
		if err := client.CleanupRepository(repo, keepLast); err != nil {
			fmt.Printf("Ошибка при очистке репозитория %s: %v\n", repo, err)
		}
	}

	fmt.Println("\n✅ Очистка завершена!")
	fmt.Println("\n⚠️  Важно: После удаления манифестов запустите garbage collection в Registry:")
	fmt.Println("docker exec <registry-container> registry garbage-collect /etc/docker/registry/config.yml")
}
