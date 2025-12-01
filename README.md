# auth_service

Небольшой микросервис аутентификации, который выдаёт краткоживущие JWT access-токены и долгоживущие refresh-токены в «сыром» виде (raw), при этом в Redis хранится только их SHA-256 хэш. Сервис предоставляет gRPC API.

**Ключевое поведение:**

* **Login (Вход):** проверка учётных данных, возврат JWT access-токена (подписанного) и сырого refresh-токена (клиент хранит сырой токен, сервер сохраняет в Redis только SHA-256 хэш).
* **Refresh (Обновление):** ротация refresh-токенов в Redis с помощью атомарной операции (Lua-скрипт).
* **Revoke (Отзыв):** удаление хэша refresh-токена из Redis.

---

## Переменные окружения

* `DB_URL` — строка подключения к Postgres (обязательно)
* `GRPC_ADDR` — адрес для gRPC-сервера (рекомендованный по умолчанию: `:50051`)
* `REDIS_ADDR` — адрес Redis (по умолчанию: `localhost:6379`)
* `SECRET_KEY` — HMAC-секрет для подписи access-токенов (должен быть минимум 32 байта)

---

## Схема базы данных

**Модель пользователя (User):**

* `id` (text, первичный ключ)
* `username` (text, уникальный, not null)
* `password` (text, bcrypt-хеш)
* `created_at` (timestamp with timezone, по умолчанию `now()`)

Миграции — SQL-файлы в папке `migrations/`.

Пример:
`0001_create_users.up.sql` / `0001_create_users.down.sql`

---

## Локальный запуск

### 1. Установи переменные окружения

```bash
export DB_URL="postgres://user:pass@localhost:5432/authdb?sslmode=disable"
export REDIS_ADDR="localhost:6379"
export SECRET_KEY="$(head -c 48 /dev/urandom | base64)"
export GRPC_ADDR=":50051"
```

### 2. Примени миграции

Через `psql`:

```bash
psql "$DB_URL" -f migrations/0001_create_users.up.sql
```

Или используй любой миграционный инструмент (migrate, goose и т.д.).

### 3. Собери и запусти

```bash
go build -o bin/auth_service ./cmd/server
GRPC_ADDR=":50051" DB_URL="$DB_URL" REDIS_ADDR="$REDIS_ADDR" SECRET_KEY="$SECRET_KEY" ./bin/auth_service
```

> **Примечание:** Если `GRPC_ADDR` пустой, сервер не сможет запуститься. Используй разумное значение по умолчанию, например `:50051`.

---

## gRPC API (proto)

Сервис: `auth.AuthService`

RPC-методы:

* `Login(LoginRequest) returns (TokenResponse)`
* `Register(RegisterRequest) returns (Status)`
* `Refresh(RefreshRequest) returns (TokenResponse)`
* `Revoke(RevokeRequest) returns (Status)`

Proto-файлы находятся в папке `proto/`, сгенерированный код уже добавлен в проект.

---

## Примеры вызовов (grpcurl)

**Регистрация:**

```bash
grpcurl -plaintext -d '{"username":"alice","password":"secret"}' localhost:50051 auth.AuthService/Register
```

**Вход (Login):**

```bash
grpcurl -plaintext -d '{"username":"alice","password":"secret"}' localhost:50051 auth.AuthService/Login
```

**Обновление токена (Refresh):**

```bash
grpcurl -plaintext -d '{"refresh_token":"<raw-refresh-token>","expected_user_id":"<user-id>"}' localhost:50051 auth.AuthService/Refresh
```

**Отзыв токена (Revoke):**

```bash
grpcurl -plaintext -d '{"refresh_token":"<raw-refresh-token>","user_id":"<user-id>"}' localhost:50051 auth.AuthService/Revoke
```

---

## Исправления багов и изменения поведения

1. Билдеры в репозитории теперь создаются на каждый вызов (исправляет сложные для отладки ошибки с переиспользованием состояния и проблемами конкурентности).
2. Введена единая ошибка хранилища — `autherr.ErrStorageError`, и все места приведены к ней.
3. Репозиторий теперь маппит “no rows” в `autherr.ErrNotFound`. Сервис возвращает `ErrNotFound` в этом случае; все остальные ошибки БД — как `ErrStorageError`.
4. `rpc.NewAuthServer` теперь возвращает реальную ошибку из инициализации token service вместо того, чтобы маскировать её.

Эти изменения уменьшают риск скрытых ошибок и утечек состояния билдера между запросами.

---

## Рекомендуемые следующие шаги

* Добавить автоматический запуск миграций (например, через golang-migrate или goose) при запуске сервиса или в CI.
* Можно сделать так, чтобы конструкторы репозиториев принимали фабрику билдера, если ожидается много реализаций; в текущем виде безопасное создание на каждый вызов — простое и надёжное решение.
* Добавить интеграционные тесты для Register/Login/Refresh/Revoke с тестовыми Postgres и Redis для проверки всей цепочки.
* Добавить структурированное логирование запросов/ответов и метрики для мониторинга проблем с токенами, БД и Redis.
