UPDATE templates
SET body = '# {{title}}

{{body}}',
	updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE name = 'wiki_page.default'
	AND body = '# {{title}}';
