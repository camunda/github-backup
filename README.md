# Github Backup

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

[] Create new user and provision S3 full access IAM

[] Create Jenkins job



