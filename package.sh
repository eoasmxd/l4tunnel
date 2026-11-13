# compile for version
make
if [ $? -ne 0 ]; then
    echo "make error"
    exit 1
fi

l4t_version=`./bin/l4ts -v`
echo "build version: $l4t_version"

# cross_compiles
make -f ./Makefile.cross-compiles

rm -rf ./packages
mkdir ./packages

os_all='linux windows'
arch_all='386 amd64 arm arm64 mips64 mips64le mips mipsle'

for os in $os_all; do
    for arch in $arch_all; do
        l4t_dir_name="l4t_${l4t_version}_${os}_${arch}"
        l4t_path="./packages/l4t_${l4t_version}_${os}_${arch}"

        if [ "x${os}" = x"windows" ]; then
            if [ ! -f "./l4tc_${os}_${arch}.exe" ]; then
                continue
            fi
            if [ ! -f "./l4ts_${os}_${arch}.exe" ]; then
                continue
            fi
            mkdir ${l4t_path}
            mv ./l4tc_${os}_${arch}.exe ${l4t_path}/l4tc.exe
            mv ./l4ts_${os}_${arch}.exe ${l4t_path}/l4ts.exe
        else
            if [ ! -f "./l4tc_${os}_${arch}" ]; then
                continue
            fi
            if [ ! -f "./l4ts_${os}_${arch}" ]; then
                continue
            fi
            mkdir ${l4t_path}
            mv ./l4tc_${os}_${arch} ${l4t_path}/l4tc
            mv ./l4ts_${os}_${arch} ${l4t_path}/l4ts
        fi  
        cp ./LICENSE ./NOTICE ${l4t_path}
        cp -rf ./conf/* ${l4t_path}

        # packages
        cd ./packages
        if [ "x${os}" = x"windows" ]; then
            zip -rq ${l4t_dir_name}.zip ${l4t_dir_name}
        else
            tar -zcf ${l4t_dir_name}.tar.gz ${l4t_dir_name}
        fi  
        cd ..
        rm -rf ${l4t_path}
    done
done
