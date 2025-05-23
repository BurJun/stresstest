# StressTest-as-a-Service (STaaS)

STaaS — это простой и расширяемый проект на Go, предназначенный для симуляции и тестирования нагрузки на HTTP-сервисы. Проект состоит из двух компонентов:

- Receiver — HTTP-сервис, принимающий множество запросов, имитирующий задержки, ошибки и собирающий метрики.
- Attacker — генератор нагрузки, отправляющий запросы на указанный адрес с заданной частотой (RPS).

---

## Для чего это нужно

- Тестирование устойчивости API и микросервисов к высокой нагрузке
- Анализ производительности и задержек
- Интеграция с Prometheus и Grafana для визуализации метрик
- Обучение и отладка стресс-сценариев

---

## Компоненты

### Receiver (нагружаемый сервис)

- Обрабатывает запросы по пути /load
- Имитирует случайные задержки и ошибки (200, 500, 503, 404)
- Поддерживает Prometheus-метрики по адресу /metrics:
  - Общее количество запросов
  - Распределение по статусам
  - Гистограмма времени ответа

### Attacker (нагружающий сервис)

- CLI-инструмент для создания HTTP-нагрузки
- Поддерживает параметры:
  - -url — адрес сервиса (по умолчанию `http://localhost:8080/load`)
  - -rps — количество запросов в секунду
  - -duration — длительность теста в секундах
- Выводит коды ответов и время отклика

---

## Пример запуска

### 1. Запуск Receiver

```bash
go run receiver/main.go
```
Receiver будет слушать на порту :8080. Метрики будут доступны на http://localhost:8080/metrics.

### 2. Запуск Attacker
   
```bash
go run attacker/main.go -url http://localhost:8080/load -rps 100 -duration 10
```
Запустит нагрузку в 100 запросов в секунду на 10 секунд.
