
if [[ -z $(which goplantuml)  ]];then
    printf "goplantuml is a requirement."
    exit 1
fi
if [[ -z $(which plantuml)  ]];then
    printf "plantuml is a requirement."
    exit 1
fi
goplantuml -recursive -output diagrams/scriptorium_uml.puml src/
sed -i '/^namespace pb {/,/^}/d; /^"pb\./d' diagrams/scriptorium_uml.puml
plantuml -tsvg diagrams/scriptorium_uml.puml
