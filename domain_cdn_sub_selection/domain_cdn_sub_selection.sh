#!/bin/bash

set -euf -o pipefail

DOMAINCOUNT=100
CDNLIST="cdn_asn.csv"
DOMAINLIST="domains_to_cdn.csv"

need_file() {
	if [[ ! -e $1 ]]
	then
		echo "error: missing file '$1'"
		exit 1
	fi
}

need_file $CDNLIST
need_file $DOMAINLIST

if [[ ! -d domain_cdn_sub_selection ]]
then
	echo "error: please exec from the root of the project dir"
	exit 1
fi

echo "Processing CDNs in '$CDNLIST' and extracting a max of '$DOMAINCOUNT' domains from '$DOMAINLIST' ... "

for cdn_name in $(cat $CDNLIST|cut -d "," -f1|tr -d '"')
do
	[[ $cdn_name == "cdn_name" ]] && continue
	OUTPUTFILE="${cdn_name}_domain_selection.txt"
	if grep "^${cdn_name}," $DOMAINLIST|shuf -n $DOMAINCOUNT > domain_cdn_sub_selection/${OUTPUTFILE} ; then
		echo "Processed $(cat domain_cdn_sub_selection/${OUTPUTFILE}|wc -l) domains for ${cdn_name} into $OUTPUTFILE"
	else
		echo "warning: no domains found for '$cdn_name'"
	fi
done
echo "Done! :D"
