# gogit
golang实现的githack,自动解析源码文件并保存，为https://github.com/lijiejie/GitHack 的golang版本
由于lijiejie大佬的githack制作解析下载，由于有时会有许多图片文件或者css文件下载，太占用空间、时间。于是加上了参数进行过滤
其次加入了许多

~~~
使用方法：gogit -u http://example.com/.git


-uf filepath 批量扫描并保存
-e string  不保存指定后缀的文件
-i string  只保存指定后缀的文件
-en string 不保存名字中带某字符串的文件
-n string 只保存名字中带指定字符串的文件
-o        是否保存解析出的所有文件的文件路径（可用于未授权扫描字典，文件保存在生成的目录中的gitAllUrl.txt中）

~~~
