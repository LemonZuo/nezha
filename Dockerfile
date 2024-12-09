FROM alpine AS runner

ARG TARGETOS
ARG TARGETARCH

RUN apk update \
    && apk upgrade \
    && apk add --no-cache ca-certificates tzdata \
    && update-ca-certificates 2>/dev/null || true

COPY ./script/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

WORKDIR /dashboard
COPY dist/dashboard-${TARGETOS}-${TARGETARCH} ./app

VOLUME ["/dashboard/data"]
EXPOSE 80 5555
ENTRYPOINT ["/entrypoint.sh"]