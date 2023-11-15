#!/bin/bash
while true
do
    if read line; then
        echo $line > /dev/stdout
    fi
done < $1
