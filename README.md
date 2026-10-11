# 尖塔工坊 SpireForge

**仅支持《杀戮尖塔 1》(Slay the Spire, Steam AppID 646570,初代)——不支持《杀戮尖塔 2》。**

杀戮尖塔 1 存档编辑器 + 开局模板生成器。Go + Fyne 编写,为 Steam Deck / Bazzite 等手柄优先的 Linux 游戏环境设计,也支持桌面键鼠操作;Windows / macOS / Linux 全平台发布。

![平台](https://img.shields.io/badge/platform-Linux-blue) ![GUI](https://img.shields.io/badge/GUI-Fyne-purple) ![语言](https://img.shields.io/badge/UI-中文-green)

## 功能

- **开局模板**:免开局直接生成定制存档(写入 `<角色>.autosave`,也可覆盖当前这一局)
  - 4 职业(战士/猎手/机器人/观者)、默认能量(3-6,自动挂能量遗物)、血量(默认 120)、初始金币、打击/防御数量
  - 每职业 3-4 套流派核心卡(力量/壁垒/消耗、剧毒/小刀/弃牌、雷电/冰霜/利爪/黑暗、姿态/神格/留牌)
  - 卡牌/起手卡/核心卡一键升级,默认 4 件 + 18 件自选稀有遗物
- **卡组编辑**:逐张添加/删除/升级/降级,一键升级全部,内置 423 张官方卡牌(带缩略图)
- **遗物与药水**:195 件遗物 / 45 种药水的添加删除(带官方图标)
- **快速数值**:血量/金币/手牌加减即改
- **高级 JSON**:整套存档原始编辑
- **一键导入 Steam 库**(写入 shortcuts.vdf)

## 手柄操作

| 按键 | 功能 |
|------|------|
| 十字键/左摇杆 | 移动 / 加减数值 |
| A | 确认 |
| B | 返回上一层 |
| X / Y | +10 / −10 |
| START | 退出 |

键盘同样可用:方向键 + 回车 + Esc。

## 存档格式

杀戮尖塔存档 = `JSON → 与 "key" 循环异或 → base64`。本工具全部本地处理,存档不出机器。

数据来源:游戏本体 jar 的官方本地化文件(423 卡/195 遗物/45 药水,含官方简中名)+ 卡牌职业分类(按 jar 内类文件包路径) + 原版贴图缩略图。

## 下载

到 [Releases](https://github.com/friddle/spireforge/releases) 下载对应平台的 zip(Windows / macOS Intel+Apple Silicon / Linux x64+ARM64),解压即用:
- Linux/macOS: `chmod +x spireforge && ./spireforge`
- 卡牌缩略图与中文字体已打包在 zip 内,解压到同目录即可

## 构建与运行

```bash
go build -o spireforge .
FYNE_FONT=/path/to/NotoSansSC.ttf ./spireforge   # 中文字体
```

依赖:Go 1.24+,gcc,libgl/x11 开发头文件(Fyne cgo 要求)。

## Bazzite/Steam Deck flatpak 安装

```bash
# 二进制构建好后:
flatpak build-init build-dir com.spireforge.SaveForge org.freedesktop.Sdk org.freedesktop.Platform 25.08
# (沙盒无 Sdk 时可手动建 build-dir/metadata)
flatpak build-finish --command=spireforge-run --socket=x11 --socket=wayland \
  --share=ipc --device=dri --device=input --filesystem=home build-dir
flatpak build-export repo build-dir
flatpak install --user ./repo com.spireforge.SaveForge
```

注意 `--device=input`:手柄 evdev 事件需要 `/dev/input` 访问权限,漏掉会导致控制器失效。

## 致谢

- 卡牌/遗物/药水数据与图片:游戏本体 (Slay the Spire © Mega Crit)
- 界面:[Fyne](https://fyne.io)
- 存档编解码思路参考社区网页版存档编辑器
