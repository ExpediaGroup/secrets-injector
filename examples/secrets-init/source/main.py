import boto3
import sys
import yaml
import json
import toml
from unflatten import unflatten
from jproperties import Properties
from ec2_metadata import ec2_metadata
client = boto3.client('secretsmanager', region_name=ec2_metadata.region)

if (len(sys.argv) == 1):
    print("No secrets to be mounted, exiting")
    sys.exit(0)

secret_names = sys.argv[1].split(",")
out_directory = sys.argv[2]
file_type = sys.argv[3]

values = {secret["Name"].replace('/', '.'): secret["SecretString"] for secret in map(
    lambda name: client.get_secret_value(SecretId=name), secret_names)}
file_name = "%s/secrets.%s" % (out_directory, file_type)

if file_type == "yaml":
    yaml.dump(unflatten(values), open(file_name, 'w'), explicit_start=True)
elif file_type == "json":
    json.dump(unflatten(values), open(file_name, 'w'))
elif file_type == "toml":
    toml.dump(unflatten(values), open(file_name, 'w'))
else:
    properties = Properties()
    properties.properties = values

    with open(file_name, "wb") as out_file:
        properties.store(out_file, strict=True)
