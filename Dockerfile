FROM golang:latest as builder

WORKDIR /app

# Kopiraj go.mod i go.sum
COPY ./lunar-gateway/go.mod ./lunar-gateway/go.sum ./

# Kopiraj lokalne module (ako ih imaš kao replace)
COPY ./oort ../oort
COPY ./magnetar ../magnetar
COPY ./apollo ../apollo
COPY ./heliosphere ../heliosphere

# Preuzmi sve zavisnosti
RUN go mod download

# Kopiraj ostatak koda
COPY ./lunar-gateway .

# Uradi tidy da bi go.mod bio ažuriran
RUN go mod tidy

# Build aplikacije
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Final stage
FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/main .
COPY --from=builder /app/config/config.yml .
COPY --from=builder /app/config/no_auth_config.yml .

EXPOSE 5555

CMD ["./main"]
