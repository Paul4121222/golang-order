DB_URL = postgres://paul:secret123@localhost:5432/orderdb?sslmode=disable

.PHONY: migration_up migrate-down migrate-status migrate-new

migration_up:
	migrate -database "$(DB_URL)" -path "$$(pwd)/migrations" up

migrate-down:
	migrate -database "$(DB_URL)" -path "$$(pwd)/migrations" down 1

migrate-status:
	migrate -database "$(DB_URL)" -path "$$(pwd)/migrations" version

migrate-new:
	@printf "migration_name: ";read name; \
	migrate create -ext sql -dir migrations sel $$name