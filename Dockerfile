ARG PYTHON_VERSION=3.12

# ---------------------------------------------------------
# Builder Stage
# ---------------------------------------------------------
FROM python:${PYTHON_VERSION}-slim-bookworm AS builder

WORKDIR /app

ENV UV_LINK_MODE=copy \
    UV_LOCKED=1 \
    UV_COMPILE_BYTECODE=1

# Install dependencies first (for layer caching)
RUN --mount=from=ghcr.io/astral-sh/uv,source=/uv,target=/bin/uv \
    --mount=type=cache,target=/root/.cache/uv \
    --mount=type=bind,source=uv.lock,target=uv.lock \
    --mount=type=bind,source=pyproject.toml,target=pyproject.toml \
    uv sync --no-install-project --no-dev

# Copy source code and install project
COPY README.md pyproject.toml uv.lock ./
COPY camply /app/camply

RUN --mount=from=ghcr.io/astral-sh/uv,source=/uv,target=/bin/uv \
    --mount=type=cache,target=/root/.cache/uv \
    uv sync --no-dev --extra all --no-editable

# ---------------------------------------------------------
# Final Runtime Stage
# ---------------------------------------------------------
FROM python:${PYTHON_VERSION}-slim-bookworm

# Copy ONLY the compiled virtual environment from the builder
COPY --from=builder /app/.venv /app/.venv

# Set up path so the virtual environment is used automatically
ENV PATH="/app/.venv/bin:${PATH}"
ENV HOME=/home/camply
ENV CAMPLY_LOG_HANDLER="PYTHON"

RUN mkdir -p ${HOME}
WORKDIR ${HOME}

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

# Generate bash completion
RUN _CAMPLY_COMPLETE=bash_source camply > ${HOME}/.camply-complete.bash && \
    echo "[[ ! -f ${HOME}/.camply-complete.bash ]] || source ${HOME}/.camply-complete.bash" >> ${HOME}/.bashrc

CMD ["camply", "--help"]
