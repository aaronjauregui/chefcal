FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /chefcal .

FROM alpine:3.20
RUN apk add --no-cache tzdata
COPY --from=build /chefcal /chefcal
ENTRYPOINT ["/chefcal"]
CMD ["-config", "/config.yaml"]
