#!/bin/bash

set -euf -o pipefail

DOMAINCOUNT=30
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

echo "Processing CDNs in '$CDNLIST' and extracting a max of '$DOMAINCOUNT' domains from '$DOMAINLIST' ... "

for cdn_name in $(cat $CDNLIST|cut -d "," -f1|tr -d '"')
do
	[[ $cdn_name == "cdn_name" ]] && continue
	OUTPUTFILE="${cdn_name}_domain_selection.txt"
	if grep "^${cdn_name}," $DOMAINLIST|cut -d "," -f2|shuf -n $DOMAINCOUNT > $OUTPUTFILE ; then
		echo "Processed $(cat $OUTPUTFILE|wc -l) domains for ${cdn_name} into $OUTPUTFILE"
	else
		echo "warning: no domains found for '$cdn_name'"
	fi
done
echo "Done! :D"
