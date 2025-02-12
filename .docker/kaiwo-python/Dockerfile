# Use the smallest official Python image (Debian-based)
FROM python:3.12-slim

# Set environment variables to reduce image size
ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1

# Install system dependencies
#RUN apt-get update && apt-get install -y --no-install-recommends \
#    gcc \
#    libffi-dev \
#    musl-dev \
#    build-essential \
#    && rm -rf /var/lib/apt/lists/*  # Clean up to reduce image size

# Install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application code
COPY kaiwo /app/kaiwo
WORKDIR /app
