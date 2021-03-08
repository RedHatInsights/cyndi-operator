#!/bin/bash
echo "---" > manifest.yaml
for file in ./bundle/manifests/*.yaml; do
	cat ${file} >> manifest.yaml
	echo "---" >> manifest.yaml
done
