# Usage: make db-reset (destroy the schema and rebuild)
db-reset:
	@docker compose down -v
	@docker compose up -d