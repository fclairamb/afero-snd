# Afero Sync & Delete fs

The S&D file system relies on:
- A temporary file system to store files as fast as possible
- A permanent file system for the destination files

It allows to quickly accept new files and synchronize them on a slower file system.

The rationale is that when using some cloud-bases file systems like [S3](https://github.com/fclairamb/afero-s3), [Google Drive](https://github.com/fclairamb/afero-gdrive) or [Dropbox](https://github.com/fclairamb/afero-dropbox) on an [FTP server](https://github.com/fclairamb/ftpserver), some devices consider the slowness as an error.

