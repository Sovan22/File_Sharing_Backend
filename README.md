# 21BPS1465_Backend
This is a file sharing backend system implemented using Go, GORM, and Redis. It supports file uploads, retrieval, sharing, deletion, and search functionality. It also includes rate limiting and caching with Redis.

## Table of Contents
- [Getting Started](#getting-started)
- [Docker Setup](#docker-setup)
- [API Endpoints](#api-endpoints)
  - [User Routes](#user-routes)
  - [File Routes](#file-routes)
- [Rate Limiting](#rate-limiting)
- [Caching](#caching)


## Getting Started

To get started with the project, follow these steps:

1. **Clone the Repository:**
    ```bash
    git clone https://github.com/Sovan22/21BPS1465_Backend.git
    cd 21BPS1465_Backend
    ```

2. **Install Dependencies:**
    Ensure you have Go and Redis installed. Then, run:
    ```bash
    go mod tidy
    ```

3. **Run the Server:**
    ```bash
    go run main.go
    ```

## Docker Setup

To run the project using Docker and Docker Compose, follow these steps:

1. **Ensure Docker and Docker Compose are installed.**

2. **Clone the Repository**

    ```bash
    git clone https://github.com/Sovan22/21BPS1465_Backend.git
    cd 21BPS1465_Backend
    ```

3. **Build and Run Docker Containers:**

    ```bash
    docker-compose up --build
    ```

4. **Access the Application:**
    The application will be available at `http://localhost:8080`.

## API Endpoints

### User Routes

- **Register a New User**
  - **Endpoint:** `POST /register`
  - **Description:** Registers a new user.
  - **Request Body:**
    ```json
    {
      "email": "user@example.com",
      "password": "password123"
    }
    ```
  - **Responses:**
    - `200 OK` - Registration successful.
    - `400 Bad Request` - Invalid input or email already registered.
    - `500 Internal Server Error` - Server error.
   
 ![register](https://github.com/user-attachments/assets/122eb2b0-3a86-4cc5-8bf4-b860936e076e)


- **Login**
  - **Endpoint:** `POST /login`
  - **Description:** Logs in a user and returns a JWT token.
  - **Request Body:**
    ```json
    {
      "email": "user@example.com",
      "password": "password123"
    }
    ```
  - **Responses:**
    - `200 OK` - Login successful with token.
    - `401 Unauthorized` - Invalid credentials.
    - `500 Internal Server Error` - Server error.

 ![login](https://github.com/user-attachments/assets/b16546b3-d7db-4817-b29d-bea05e8c8f34)


### File Routes

- **Upload File**
  - **Endpoint:** `POST /upload`
  - **Description:** Uploads a file.
  - **Request Body:** Form data with file upload.
  - **Responses:**
    - `200 OK` - File uploaded successfully.
    - `400 Bad Request` - Failed to get file from request.
    - `500 Internal Server Error` - Failed to save file or metadata.
 ![upload](https://github.com/user-attachments/assets/33d569e7-8937-4a10-9612-e7a96467d466)

- **Get User Files**
  - **Endpoint:** `GET /files`
  - **Description:** Retrieves a list of files uploaded by the authenticated user.
  - **Responses:**
    - `200 OK` - Returns a list of files.
    - `500 Internal Server Error` - Failed to retrieve files.
 ![getfiles](https://github.com/user-attachments/assets/a7396db5-b315-49e2-8bfa-58a6872a5f50)

- **Share File**
  - **Endpoint:** `GET /share/:fileID`
  - **Description:** Generates a shareable link for a file.
  - **Query Parameters:**
    - `fileID` - ID of the file to share.
    - `expiry` - Expiry time for the shareable link (optional) "1h", "30m" ,"1h30m".
  - **Responses:**
    - `200 OK` - Returns the shareable URL.
    - `400 Bad Request` - Invalid file ID.
    - `404 Not Found` - File not found.
    - `500 Internal Server Error` - Server error.
  ![share](https://github.com/user-attachments/assets/8efd44f9-9187-4c43-bc31-06db09665667)

- **Delete File**
  - **Endpoint:** `GET /delete/:fileID`
  - **Description:** Deletes a file by ID.
  - **Query Parameters:**
    - `fileID` - ID of the file to delete.
  - **Responses:**
    - `200 OK` - File deleted successfully.
    - `400 Bad Request` - Invalid file ID.
    - `404 Not Found` - File not found.
    - `500 Internal Server Error` - Failed to delete file.
  ![delete](https://github.com/user-attachments/assets/94197851-1bb4-4c56-bc95-f35a93d8e2bd)


- **Search Files**
  - **Endpoint:** `GET /search`
  - **Description:** Searches for files based on name, type, and uploaded date.
  - **Query Parameters:**
    - `name` - Partial name of the file.
    - `type` - Type of the file (e.g., pdf).
    - `date` - Uploaded date in format YYYY-MM-DD.
    - `limit` - Number of results to return (optional).
    - `offset` - Pagination offset (optional).
  - **Responses:**
    - `200 OK` - Returns search results.
    - `400 Bad Request` - Invalid date format.
    - `500 Internal Server Error` - Failed to search files.
 ![search](https://github.com/user-attachments/assets/18b9bdb4-60da-434b-9dc1-7b6179a7acca)

## Rate Limiting

To prevent abuse, the API enforces rate limiting:
- **Limit:** 100 requests per user per minute.

## Caching

- **File Metadata Caching:** Cached using Redis to reduce database load. The cache expires after 5 minutes.
  

