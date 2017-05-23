# marathon-daemonset
Handles updating Marathon app instances to match the number of Mesos Agents or to match the number of Mesos Agents which have matching attribute key/value pairs.


## Example env vars:

```
DAEMONSET_DRYRUN=true 
DAEMONSET_MESOSHOST=http://master.mesos:5050 
DAEMONSET_MARATHONHOST=http://marathon.mesos:8080
DAEMONSET_SERVERPORT=1234
DAEMONSET_DEBUG=true
DAEMONSET_UPDATEFREQUENCY=1m30s
```

update-frequency is the time in seconds between process runs.

## Example Marathon labels:

```
# Make sure there are the same number of instances as there are mesos agents.
"daemonset": "all"

# Make sure there are the same number of instances as there are mesos agents with an attribute pair of tier=private.
"daemonset": "tier|private"

# Make sure there are the same number of instances as there are mesos agents with an attribute pair of az=a.
"daemonset": "az|a"

# Make sure there are the same number of instances as there are mesos agents with an attribute pair of tier=private or tier=public or tier=badgers.
"daemonset": "tier|private,tier|public,tier|badgers"
```


## Running the app

```
DAEMONSET_DRYRUN=true DAEMONSET_MESOSHOST=http://master.mesos:5050 DAEMONSET_MARATHONHOST=http://marathon.mesos:8080 marathon-daemonset
```

## Marathon constraint to handle multiple tiers

```
hostname:UNIQUE
tier:LIKE:(prometheus|public|badgers)
```

