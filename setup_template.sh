#!/bin/bash

REMOTE_URL=$(git config --get remote.origin.url)
if [ $? -ne 0 ]; then
    echo "Error with git repo or retrieving remote URL"
    exit $?
fi

REPO_NAME=$(echo $REMOTE_URL | cut -c28- | rev | cut -c5- | rev)

perl -p -i -e "s/GoBasic/$REPO_NAME/g" go.mod
perl -p -i -e "s/goBasicTemplate/$REPO_NAME/g" Taskfile.yml

rm README.md
printf "# $REPO_NAME\\n" >> README.md

rm setup_template.sh
