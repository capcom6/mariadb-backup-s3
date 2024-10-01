# MariaDB Backup to S3

This project automates the process of backing up a MariaDB database, compressing the backup, and uploading it to an S3-compatible storage service.

## Features

- Backup MariaDB databases using `mariabackup`.
- Compress backups into a `.tar.gz` file.
- Upload compressed backups to S3-compatible storage.

## Prerequisites

- Go 1.22 or later
- MariaDB server
- AWS credentials for S3-compatible storage

## Installation

1. Clone the repository:
    ```shell
    git clone https://github.com/capcom6/mariadb-backup-s3.git
    cd mariadb-backup-s3
    ```

2. Build the project:
    ```shell
    go build
    ```

3. Create a `.env` file in the root directory with the following content:
    ```dotenv
    # AWS Access Key for authenticating AWS services
    AWS_ACCESS_KEY=000000000000000000000

    # AWS region where the services are hosted
    AWS_REGION=us-east-1

    # AWS Secret Key for authenticating AWS services
    AWS_SECRET_KEY=000000000000000000000

    # MariaDB username for database authentication
    MARIADB__USER=root

    # MariaDB password for database authentication
    MARIADB__PASSWORD=000000

    # URL for storage service, including bucket name and endpoint
    STORAGE__URL=s3://bucket-name/prefix?endpoint=http://127.0.0.1:9000&force_path_style=true
    ```

## Usage

Run the backup process with the following command:
```shell
./mariadb-backup-s3
```

The process will:

1. Create a temporary directory.
2. Backup the MariaDB database to the temporary directory.
3. Prepare the backup.
4. Compress the prepared backup into a .tar.gz file.
5. Upload the compressed backup to the S3-compatible storage.

## Configuration

The configuration is loaded from environment variables and command-line flags. Environment variables are defined in the `.env` file. Command-line flags can override the environment variables.

## Environment Variables

* `MARIADB__USER`: MariaDB username for database authentication.
* `MARIADB__PASSWORD`: MariaDB password for database authentication.
* `STORAGE__URL`: URL for storage service, including bucket name and endpoint. Supported query-params:
  * `endpoint`: Endpoint for the storage service.
  * `force_path_style`: Force path-style URLs.
  * `disable_delete_objects`: Use one-by-one delete instead of bulk delete.
* `BACKUP__LIMITS__MAX_COUNT`: Maximum number of backups to keep.

## Command-Line Flags

* `--db-user`: Override the MariaDB username.
* `--db-password`: Override the MariaDB password.
* `--storage-url`: Override the storage URL.

## License

This project is licensed under the Apache 2.0 License. See the [LICENSE](LICENSE) file for details.

## Contributing

1. Fork the repository.
2. Create a new branch (git checkout -b feature-branch).
3. Commit your changes (git commit -am 'Add new feature').
4. Push to the branch (git push origin feature-branch).
5. Create a new Pull Request.
