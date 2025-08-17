import React, { ReactNode } from "react";
import { cn } from "../utils";

interface ContainerProps {
  children: ReactNode;
  className?: string;
}

function Container({ children, className }: Readonly<ContainerProps>) {
  return <div className={cn("mx-auto h-full w-full px-8", className)}>{children}</div>;
}

interface ArticleProps {
  children: React.ReactNode;
}

function Article({ children }: Readonly<ArticleProps>) {
  return (
    <Container>
      <div className="grid w-full grid-cols-12">
        <div className="col-span-12 xl:col-span-11 xl:col-start-2">{children}</div>
      </div>
    </Container>
  );
}

// Create a compound component that React Fast Refresh can recognize
const ContainerWithSubcomponents = Container as typeof Container & {
  Article: typeof Article;
};

ContainerWithSubcomponents.Article = Article;

export default ContainerWithSubcomponents;