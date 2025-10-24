docker build -t aimfaker:dev-latest .
kind load docker-image aimfaker:dev-latest --name kaiwo-test
