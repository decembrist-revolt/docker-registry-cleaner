# Настройка Docker Registry для поддержки удаления образов

## Проблема
Ошибка `405 Method Not Allowed` с сообщением `"The operation is unsupported"` означает, что ваш Docker Registry не настроен для поддержки удаления образов.

## Решение

### 1. Для Docker Registry в контейнере

Если Registry запущен в Docker контейнере, создайте или обновите файл конфигурации:

```yaml
# config.yml
version: 0.1
log:
  fields:
    service: registry
storage:
  cache:
    blobdescriptor: inmemory
  filesystem:
    rootdirectory: /var/lib/registry
  delete:
    enabled: true  # ← Это ключевая настройка!
http:
  addr: :5000
  headers:
    X-Content-Type-Options: [nosniff]
health:
  storagedriver:
    enabled: true
    interval: 10s
    threshold: 3
```

Затем перезапустите Registry с новой конфигурацией:

```bash
# Остановите текущий Registry
docker stop registry-container

# Запустите с новой конфигурацией
docker run -d \
  --name registry \
  -p 5000:5000 \
  -v /path/to/config.yml:/etc/docker/registry/config.yml \
  -v registry-data:/var/lib/registry \
  registry:2
```

### 2. Для Docker Compose

Обновите ваш `docker-compose.yml`:

```yaml
version: '3'
services:
  registry:
    image: registry:2
    ports:
      - "5000:5000"
    environment:
      REGISTRY_STORAGE_DELETE_ENABLED: "true"  # ← Альтернативный способ
    volumes:
      - registry-data:/var/lib/registry
      # Или используйте файл конфигурации:
      # - ./config.yml:/etc/docker/registry/config.yml

volumes:
  registry-data:
```

### 3. Переменные окружения

Альтернативно, можно использовать переменные окружения:

```bash
docker run -d \
  --name registry \
  -p 5000:5000 \
  -e REGISTRY_STORAGE_DELETE_ENABLED=true \
  -v registry-data:/var/lib/registry \
  registry:2
```

### 4. Проверка настройки

После перезапуска Registry проверьте, что настройка применилась:

```bash
# Запустите программу снова
./registry-cleaner.exe
```

Если настройка корректна, вы не увидите предупреждение о неподдерживаемом удалении.

### 5. Важные замечания

⚠️ **Безопасность**: Включение удаления делает Registry менее безопасным. Убедитесь, что доступ к Registry ограничен.

⚠️ **Backup**: Сделайте резервную копию данных Registry перед включением удаления.

⚠️ **Garbage Collection**: После удаления манифестов обязательно запустите garbage collection для освобождения места:

```bash
docker exec <registry-container> registry garbage-collect /etc/docker/registry/config.yml
```

### 6. Альтернативные решения

Если вы не можете изменить настройки Registry:

1. **Только анализ**: Программа покажет, какие образы будут удалены, без фактического удаления
2. **Ручное удаление**: Используйте команды docker для удаления образов локально
3. **Альтернативные Registry**: Рассмотрите использование Harbor, Nexus или другого Registry с лучшей поддержкой управления

## Дополнительная информация

- [Официальная документация Docker Registry](https://docs.docker.com/registry/configuration/)
- [Docker Registry HTTP API](https://docs.docker.com/registry/spec/api/)
