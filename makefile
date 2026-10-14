# Usage: make db-reset (destroy the schema and rebuild)
db-reset:
	@docker compose down
	@docker compose up