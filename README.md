# Github Backup

Small utility which will concurrently backup many GitHub repositories across multiple organizations to S3 bucket.

## Notes

- If running in container make sure that container is not read-only.
- Make sure that bucket exists on S3.
- Make sure that git auth keys are configured and that they have access to repositories.

## Setup

1) Copy .env-example to .env (make sure it is in the same directory as the binary) ```cp .env-example .env```
2) Put all necessary secrets to .env file
3) Put all organisations name into config.yml file

## Usage

1) Build the binary with ```make build```
2) Execute the binary ```./ghbackup``` or run it with go runtime ```make run```

## TODO

[] Create cleanup on S3
    - last 7 days
    - every week after 7 days just keep last one
    - every month last week

[] Create Jenkins job


## Troubleshooting

* I'm getting ```panic: AccessDenied: Access Denied``` while trying to push file to S3, what should I do?
This backup utility does not do bucket management at the moment. Default behaviour of S3 is to enlist the contents of the bucket in case if it cannot find
key it is searching. Most likely this means that bucket does not exists. Create the S3 bucket you have specified in .env file and try again.

* Git is giving me ```exit status 128```, what should I do?
Scream & Run. (Most likely there are deleted repositories which we are trying to download. Copy paste the command from output and check.)


