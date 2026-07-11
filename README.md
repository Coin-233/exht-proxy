# exhentai 代理

代理 ex 方便没有 ex 站的用户浏览，默认屏蔽账号相关交互功能，并加入了一些小功能

## 启动

```bash
PORT=<PORT> ./exht-proxy
```

## 显示日志

```bash
SHOW_LOG=1 PORT=<PORT> ./exht-proxy
```

## 配置项

请在`.env`文件中写入cookie, 并按需配置屏蔽项目

```env

BLOCKED_PATHS=archiver.php,mytags,repo/torrent_post.php,gallerytorrents.php,uconfig.php,favorites.php
BLOCKED_QUERY_KEYS=report,act,inline_set
BLOCKED_METHODS=rategallery,votecomment,favorite,taggallery

// 1 为完全阻止种子下载, 2 为下载无跟踪的种子, 无法下载 ex 站内的
TORRENT_MODE=

// 1 为完全组织归档下载, 2 为仅允许下载 Free 文件, 已屏蔽 H@H 下载
ARCHIVER_MODE=

COOKIES='ipb_member_id= ;ipb_pass_hash= ;hath_perks= ;sk= '
```

## 功能

- 访问统计 (根据 IP 与浏览器指纹)

![!](.image/func1浏览统计.png)

- 自定义移动视图 (画廊列表、详情、浏览界面)

<details>

<summary>画廊列表</summary>

![!](.image/func2画廊列表.png)

</details>

<details>

<summary>画廊详情</summary>

| 画廊详情图 1 | 画廊详情图 2 |
| :---: | :---: |
| ![!](.image/func3.1画廊详情页1.png) | ![!](.image/func3.2画廊详情页2.png) |

</details>

<details>

<summary>浏览界面</summary>

| 浏览器图1 | 浏览器图2 |
| :---: | :---: |
| ![!](.image/func4.1浏览器图1.png) | ![!](.image/func4.2浏览器图2.png) |

</details>

### 已默认屏蔽

- 画廊下载(压缩 / 种子)
- 发布种子
- 用户设置
- 收藏
- 用户tag
- 举报画廊
- 评分(tag / 评论 / 画廊)
- 更改列表显示方式
- 评论

## 注意事项

每次启动时自动获取`igneous`, 请在欧美IP机器上部署

## TODO

- [x] 持久化访问统计
- [ ] 密码访问
- [ ] 速率限制
- [x] 网页界面汉化 & 搜索补全 (来自 [EhTagTranslation](https://github.com/EhTagTranslation/Database) )
- [ ] 个人收藏
- [ ] 允许使用一定 GP 进行归档下载 