# Muzz backend exercise

The code builds but there are still errors to iron out before it's functional, and some areas to build upon. However, the entire implementation is laid out, so you can see my approach!

Please see code comments in `main.go` for commentary.

The service can be run with `go run .`.

The database used is PostgreSQL. The service connects to

    postgresql://postgres:password@localhost:5432/postgres

This could be made available with Docker by

    docker run --name muzz-postgres -e POSTGRES_PASSWORD=password -p 127.0.0.1:5432:5432 postgres:alpine
