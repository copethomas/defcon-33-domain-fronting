# defcon-2025-domain-fronting
Defcon 2025 Malware Village Domain Fronting Talk


## CDN List

Collected through various online resources, including

https://hackertarget.com/as-ip-lookup/

look up the company name and get all the ASN numbers

download and convert to csv with ` csvkit`

`in2csv -K 1 ~/Downloads/Autonomous\ System\ Lookup\ \(AS\ \ ASN\ \ IP\)\ \ HackerTarget.com.xlsx |grep "^AS"
`

then use a script to get all the IP address
