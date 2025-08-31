# ARO-HCP Embedder Container Image
FROM python:3.11-slim

# Set working directory
WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    postgresql-client \
    libpq-dev \
    python3-dev \
    build-essential \
    git \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements first for better layer caching
COPY requirements.txt .

# Install Python dependencies
RUN pip install --no-cache-dir --upgrade pip setuptools wheel && \
    pip install --no-cache-dir -r requirements.txt

# Copy application code
COPY embedding_generator.py .
COPY mcp_server.py .

# Create non-root user for security
RUN useradd --create-home --shell /bin/bash embedder && \
    chown -R embedder:embedder /app
USER embedder

# Set environment variables
ENV PYTHONUNBUFFERED=1
ENV PYTHONDONTWRITEBYTECODE=1
ENV PORT=8000
ENV HOST=0.0.0.0

# Expose HTTP port
EXPOSE 8000

# Default to HTTP MCP server (can be overridden)
ENTRYPOINT ["python", "mcp_server.py"]



