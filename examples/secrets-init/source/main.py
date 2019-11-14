import boto3
import sys
import yaml
import json
import toml
from unflatten import unflatten
from jproperties import Properties
from ec2_metadata import ec2_metadata

if (len(sys.argv) == 1):
    print("No secrets to be mounted, exiting")
    sys.exit(0)

secret_names = sys.argv[1].split(",")
out_directory = sys.argv[2]
file_type = sys.argv[3]

#for development or debbug get allow to pass the region via CLI
if len(sys.argv) >= 5 :
  region_name = sys.argv[4]
else:
   region_name = ec2_metadata.region

client = boto3.client('secretsmanager', region_name=region_name)

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
    with open(file_name, "w") as out_file:
      for v in values.values():
        out_file.write('{}\n'.format(v))
