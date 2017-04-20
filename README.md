# marathon-daemonset
Handles updating Marathon app instances to match the number of Mesos Agents or to match the number of Mesos Agents which have matching attribute key/value pairs.


## Example configuration:

```
mesos-host: "http://mesos:5050"
marathon-host: "http://marathon:8080"
metrics-port: 8889
update-frequency: 60
```

update-frequency is the time in seconds between process runs.

## Example Marathon labels:

```
"daemonset": "all"
"daemonset": "tier|private"
"daemonset": "az|a"
```

The above examples will (assuming there is a hostname UNIQUE constraint in the app configuration):

1. Make sure there are the same number of instances as there are mesos agents.
2. Make sure there are the same number of instances as there are mesos agents with an attribute pair of tier=private.
3. Make sure there are the same number of instances as there are mesos agents with an attribute pair of az=a.


## Running the app

```
marathon-daemonset -c config.yml
```