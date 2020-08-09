# DatahandlerCLI
This is a simple cli application to upload files into the BioDataDB.

The implementation at the moment very rudimentary and mainly used for testing purposes. Additional features will be added in the future.

Missing features:
- Uploading multiple files
- Parallelize file upload

## Usage
At first an api token with sufficient rights is required to be generated from the website which in turn requires an oauth2 account.
Then a dataset version has to be created which the data object is associated with. 

In addition a config file has to be provided, that needs to be formed like this:
```yaml
Config:
  GRPCEndpoint:
    Host: localhost
    Port: 9000
```
- Config.GRPCEndpoint.Host is the hostname to access the BioDataDB backend
- Config.GRPCEndpoint.Post is the port to access the BioDataDB backend


Example:
```shell
./datahandlercli upload -c <configfilepath> -d <datasetversion> -f <filepath> -token <api token>
```

In addition a config file has to be provie