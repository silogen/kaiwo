# Use the smallest official Python image (Debian-based)
FROM python:3.12-slim

# Set environment variables to reduce image size
ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    && rm -rf /var/lib/apt/lists/*  # Clean up to reduce image size

# Install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy test data to enable uploading mock test data with this image
COPY test/data /app/test/data
COPY test/upload_test_data.py /app/test/upload_test_data.py

# Copy application code
COPY kaiwo /app/kaiwo
WORKDIR /app
