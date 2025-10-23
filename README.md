# 启动

```bash
uvicorn app:app --host 0.0.0.0 --port <PORT>
```

## 显示日志
```bash
SHOW_LOG=1 uvicorn app:app --host 0.0.0.0 --port <PORT>
```

请在`.env`文件中写入cookie, 并按需配置屏蔽项目

### TODO
- 持久化访问统计
- 密码访问
- 速率限制
