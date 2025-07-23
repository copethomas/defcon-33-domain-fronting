# defcon-2025-domain-fronting

Defcon 2025 Malware Village Domain Fronting Talk

Building on the great work of [Karthika Subramani](https://karthikas03.github.io/):
- Paper ~ [Discovering and Measuring CDNs Prone to Domain Fronting (2024)](https://doi.org/10.1145/3589334.3645656)
- Git repo https://github.com/karthikaS03/DomainFrontingDiscovery

Within this repo I improve on the original CDN detection methods by using only open source datasets
and using an ASN -> IP -> CDN lookup system based on DNS resolution.

## 1. CDN List (Manual Process)

To rank and test a collection of CDNs we need a list of CDNs! 

This was gathered through some Googling and manual research
and then [PeeringDB](https://www.peeringdb.com/net/906) and [HackerTarget](https://hackertarget.com/as-ip-lookup/) were used
to associate each CDN with their ASN number. 

We store this list in `cdn_asn.csv`

Data Collection Date: 2025-07-06

## 2. ASN to IP Map

We then use the `cdn-asn-ip-map` Go program to covert all the CDN ASN numbers to a collection of IP ranges
which we dump to the `cdn_asn_to_ip_map.json` file. 
This file is used in later programs as a lookup table.
(The free https://iptoasn.com tab separated database is used to perform the lookup)

```shell
go run cmd/cdn-asn-ip-map/main.go
```

## 3. Prepare domain list

We now need a large number of sites to test which CDN they are associated with:
- https://tranco-list.eu/ - For the top 1 million domains

## 4. Scrape domains and analyze CDN usage

The `resolve` tool processes the list of domains and then uses the returned DNS data to perform a CDN look up
using the `cdn_asn_to_ip_map.json` mapping file. All of this data is then logged to `domains_to_cdn.csv`.

```shell
go run cmd/resolve/main.go
```

![resolve example](assets/img/resolve_progress.png "Resolve example")



## 5. (Optional) Split the domain list into a smaller CDN selection

One million is a quite large number of domains so we can run a simple script to cut down the number we are going to test
to save time on web scraping.

```bash
./domain_cdn_sub_selection/domain_cdn_sub_selection.sh 2>&1 | tee -a domain_cdn_sub_selection/domain_cdn_sub_selection.log
```

<details>
<summary>Output</summary>

```bash
Processing CDNs in 'cdn_asn.csv' and extracting a max of '30' domains from 'domains_to_cdn.csv' ... 
Processed 30 domains for Akamai into Akamai_domain_selection.txt
Processed 30 domains for Alibaba_Cloud into Alibaba_Cloud_domain_selection.txt
Processed 30 domains for Amazon_CloudFront into Amazon_CloudFront_domain_selection.txt
warning: no domains found for 'Aryaka'
Processed 30 domains for Baidu into Baidu_domain_selection.txt
Processed 18 domains for BelugaCDN into BelugaCDN_domain_selection.txt
Processed 30 domains for BlazingCDN into BlazingCDN_domain_selection.txt
Processed 30 domains for Bunny.net into Bunny.net_domain_selection.txt
Processed 16 domains for BytePlus into BytePlus_domain_selection.txt
Processed 9 domains for CacheFly into CacheFly_domain_selection.txt
Processed 30 domains for CDN77 into CDN77_domain_selection.txt
Processed 15 domains for CDNetworks into CDNetworks_domain_selection.txt
Processed 30 domains for Cloudflare into Cloudflare_domain_selection.txt
Processed 30 domains for Comcast_Technology_Solutions into Comcast_Technology_Solutions_domain_selection.txt
warning: no domains found for 'Edgio'
Processed 30 domains for EdgeNext into EdgeNext_domain_selection.txt
Processed 30 domains for Fastly into Fastly_domain_selection.txt
warning: no domains found for 'Cedexis'
warning: no domains found for 'Datum'
Processed 30 domains for G-Core_Labs into G-Core_Labs_domain_selection.txt
Processed 30 domains for GlobalConnect into GlobalConnect_domain_selection.txt
Processed 30 domains for Google_Cloud_CDN into Google_Cloud_CDN_domain_selection.txt
Processed 30 domains for Huawei_Cloud into Huawei_Cloud_domain_selection.txt
Processed 30 domains for Imperva_CDN into Imperva_CDN_domain_selection.txt
Processed 23 domains for adobe into adobe_domain_selection.txt
Processed 10 domains for cdnvideo into cdnvideo_domain_selection.txt
Processed 4 domains for KeyCDN into KeyCDN_domain_selection.txt
Processed 30 domains for Lumen into Lumen_domain_selection.txt
Processed 1 domains for MainStreaming into MainStreaming_domain_selection.txt
Processed 10 domains for Medianova into Medianova_domain_selection.txt
Processed 30 domains for Microsoft_Azure_CDN into Microsoft_Azure_CDN_domain_selection.txt
warning: no domains found for 'Netskrt'
Processed 30 domains for Ngenix into Ngenix_domain_selection.txt
warning: no domains found for 'Qwilt'
Processed 30 domains for GoDaddy into GoDaddy_domain_selection.txt
Processed 30 domains for Tata_Communications into Tata_Communications_domain_selection.txt
Processed 30 domains for Tencent into Tencent_domain_selection.txt
warning: no domains found for 'Velocix'
Processed 30 domains for Wangsu into Wangsu_domain_selection.txt
Processed 30 domains for wixdns into wixdns_domain_selection.txt
warning: no domains found for 'Yottaa'
Done! :D
```
</details>

(Optional)

Combine all the subsections into a new macro-selection

```bash
echo "cdn,domain_sld,ip_addr" > domains_to_cdn_macro_selection.csv && cat domain_cdn_sub_selection/*.txt >> domains_to_cdn_macro_selection.csv
```

## 5. Feed data into Karthika Subramani DomainFrontingDiscovery tooling

Clone Patched tooling 

```bash
git clone git@github.com:copethomas/DomainFrontingDiscovery.git
```

Update inputs and outputs in the config file:


```
vi src/config.ini

[FILE_PATHS]
crawling_results_path = <repo_location>/data/crawler_results
cdn_domain_mapping_file_path = <defcon-2025-domain-fronting_repo_location>/domains_to_cdn_macro_selection.csv (or the full seletion)
```

Create python env and install packages:
```bash
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt
```

Change working dir:
```bash
cd src/crawler_module
```

Run the Crawler:
```bash
../../.venv/bin/python3 crawl_urls.py | tee -a crawler_module.log
```

### Disadvantages

DNS based looksups, you get different responses depending on where you query and how. On the command line I was getting samsung servers and in the browser I was getting akamai :(

### Notes

Most of the code in this repo was generated by AI. It should be taken with a pinch of salt.
