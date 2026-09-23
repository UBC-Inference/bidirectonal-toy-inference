FROM ghcr.io/astral-sh/uv:0.10.10 AS uv

FROM python:3.12-slim

COPY --from=uv /uv /uvx /bin/
WORKDIR /app

COPY pyproject.toml uv.lock ./
RUN uv sync --locked --no-dev

COPY main.py ./

ENV PATH="/app/.venv/bin:$PATH"
EXPOSE 8000
CMD ["python", "main.py"]
