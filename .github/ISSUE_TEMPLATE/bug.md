name: "问题反馈"
description: "反馈问题"
labels: [· Bug, 新反馈]
body:
- type: checkboxes
  id: "yml-1"
  attributes:
    label: "检查项"
    description: "请逐个检查下列项目，并勾选确认。"
    options:
    - label: "我已确认不是网络问题"
      required: true
    - label: "我已经确认的版本为**最新**"
      required: true
    - label: "我已在 [Issues 页面](https://github.com/jdnjk/AutoInstall/issues?q=is%3Aissue+) 搜索这一 Bug 未被提交过。"
      required: true
- type: textarea
  id: "yml-2"
  attributes:
    label: 描述
    description: "详细描述具体表现。"
  validations:
    required: true
- type: textarea
  id: "yml-3"
  attributes:
    label: 本地日志
    description: "上传本地日志"
    placeholder: "先点击这个文本框，然后再将文件直接拖拽到文本框中以上传。"
  validations:
    required: true
