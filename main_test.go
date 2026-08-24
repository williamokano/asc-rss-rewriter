package main

import (
	"strings"
	"testing"
)

func TestRewriteRSS(t *testing.T) {
	input := `<rss version="2.0">
<channel>
<generator>ASC RSS 2.0</generator>
<language>en</language>
<title>ASC</title>
<description>ASC RSS Feed</description>
<link>https://cliente.amigos-share.club</link>
<copyright>Copyright ASC</copyright>
<pubDate>Mon, 24 Aug 2026 21:42:58 +0000</pubDate>
<item>
<title>A Morte do Demônio: Em Chamas (Evil Dead Burn)</title>
<guid>https://cliente.amigos-share.club/torrents-details.php?id=179278&hit=1</guid>
<link>https://cliente.amigos-share.club/torrents-details.php?id=179278&hit=1</link>
<pubDate>2026-08-24 21:13:05</pubDate>
<category> Filmes: </category>
<description>Categoria: Filmes: Tamanho: 20.90 GB Added: 119 Seeders: 3 Leechers: 2026-08-24 21:13:05</description>
</item>
<item>
<title>A Guerra do Fogo (Quest for Fire)</title>
<guid>https://cliente.amigos-share.club/torrents-details.php?id=179275&hit=1</guid>
<link>https://cliente.amigos-share.club/torrents-details.php?id=179275&hit=1</link>
<pubDate>2026-08-24 18:41:39</pubDate>
<category> Filmes: </category>
<description>Categoria: Filmes: Tamanho: 3.48 GB Added: 119 Seeders: 2 Leechers: 2026-08-24 18:41:39</description>
</item>
<item>
<title>Nosso Amor de Ontem (The Way We Were)</title>
<guid>https://cliente.amigos-share.club/torrents-details.php?id=179274&hit=1</guid>
<link>https://cliente.amigos-share.club/torrents-details.php?id=179274&hit=1</link>
<pubDate>2026-08-24 18:38:42</pubDate>
<category> Filmes: </category>
<description>Categoria: Filmes: Tamanho: 4.36 GB Added: 119 Seeders: 0 Leechers: 2026-08-24 18:38:42</description>
</item>
</channel>
</rss>`

	output := rewriteRSS(input)

	// We expect the original channel link to remain unchanged
	if !strings.Contains(output, "<link>https://cliente.amigos-share.club</link>") {
		t.Errorf("Expected output to contain the original channel link, but it didn't")
	}

	// We expect the 1st item link to be rewritten
	expectedItem1Link := `<link>https://cliente.amigos-share.club/download.php?id=179278</link>`
	if !strings.Contains(output, expectedItem1Link) {
		t.Errorf("Expected output to contain %s, but it didn't", expectedItem1Link)
	}

	// We expect the 1st item to have an enclosure tag
	expectedItem1Enclosure := `<enclosure url="https://cliente.amigos-share.club/download.php?id=179278" type="application/x-bittorrent"/>`
	if !strings.Contains(output, expectedItem1Enclosure) {
		t.Errorf("Expected output to contain %s, but it didn't", expectedItem1Enclosure)
	}

	// We expect the 2nd item link to be rewritten
	expectedItem2Link := `<link>https://cliente.amigos-share.club/download.php?id=179275</link>`
	if !strings.Contains(output, expectedItem2Link) {
		t.Errorf("Expected output to contain %s, but it didn't", expectedItem2Link)
	}

	// We expect the 3rd item link to be rewritten
	expectedItem3Link := `<link>https://cliente.amigos-share.club/download.php?id=179274</link>`
	if !strings.Contains(output, expectedItem3Link) {
		t.Errorf("Expected output to contain %s, but it didn't", expectedItem3Link)
	}
}

func TestFixInvalidXML(t *testing.T) {
	input := `<guid>https://site.com/?id=1&hit=1</guid><title>A & B &amp; C &lt; D</title>`
	expected := `<guid>https://site.com/?id=1&amp;hit=1</guid><title>A &amp; B &amp; C &lt; D</title>`
	
	output := fixInvalidXML(input)
	if output != expected {
		t.Errorf("Expected %s, got %s", expected, output)
	}
}
