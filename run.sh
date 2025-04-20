if [[ -n $MS_CONFIG_DATA ]]; then
    echo $MS_CONFIG_DATA | base64 -d > ms_config.json
else
    echo "Missing MS_CONFIG_DATA env var"
    exit 1
fi
/app/os-serverlist-sync