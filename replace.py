import re

NEW_FOOTER = '''
<div class="dp">
    <a href="/">Front</a>
    &nbsp; 本网站为 <a href="https://exhentai.org" target="_blank">https://exhentai.org</a> 代理, 仅供预览
    &nbsp; <a href="https://github.com/Coin-233/exht-proxy" target="_blank">GitHub</a>
</div>
'''.strip()


def replace_footer(html: str) -> str:

    pattern = re.compile(r'<div\s+class=["\']dp["\'][^>]*>.*?</div>',
                         re.DOTALL | re.IGNORECASE)
    if pattern.search(html):
        return pattern.sub(NEW_FOOTER, html)
    else:
        return html + "\n" + NEW_FOOTER
